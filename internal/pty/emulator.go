// Package pty provides tools for terminal emulation (PTY)
// with the ability to intercept, parse ANSI sequences, and render state.
package pty

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// PTY describes a minimal interface for working with a pseudo-terminal.
type PTY interface {
	io.ReadWriteCloser
	// Resize changes the physical dimensions of the terminal.
	Resize(cols, rows int) error
	// Pid returns the PID of the process running in the PTY.
	Pid() int
	// WaitContext waits for the process to terminate or the context to be canceled.
	WaitContext(ctx context.Context) (uint32, error)
}

// Cell represents one indivisible cell on the terminal screen.
type Cell struct {
	Rune rune   // The character displayed in the cell
	FG   string // The current ANSI foreground color sequence
	BG   string // The current ANSI background color sequence
}

// TerminalState stores the current visual state of the emulated screen.
type TerminalState struct {
	Rows, Cols int      // Terminal grid dimensions
	Screen     [][]Cell // Two-dimensional array of cells (canvas)
	CursorX    int      // Current horizontal cursor position (0...Cols-1)
	CursorY    int      // Current vertical cursor position (0...Rows-1)
	CurFG      string   // Active text color for new characters
	CurBG      string   // Active background color for new characters
}

// EmulatedPTY wraps the standard PTY,
// implements stream parsing of incoming data,
// and maintains the current state of the "virtual screen."
type EmulatedPTY struct {
	inner  PTY
	state  *TerminalState
	mu     sync.RWMutex // Protects access to the parser state and buffers
	output *io.PipeWriter
	input  *io.PipeReader

	closed chan struct{} // Signals the completion of readerLoop
	err    error         // Stores the error that caused the PTY to close.

	parserState int    // The current state of the parser's state machine
	csiBuf      []byte // Buffer for accumulation of CSI sequence parameters
}

// State machines for parsing ANSI/VT100 sequences.
const (
	stateNormal    = iota // Normal text input
	stateEscape           // Encountered an ESC character (\x1b)
	stateCSI              // Encountered a Control Sequence Introducer (ESC [)
	stateOSC              // Encountered an Operating System Command (ESC ]), used to ignore metadata
	stateOSCEscape        // Encountered an ESC an OSC
)

// NewEmulatedPTY creates a terminal emulator instance with the specified grid.
// Automatically starts a background goroutine to read from the PTY.
func NewEmulatedPTY(inner PTY, rows, cols int) *EmulatedPTY {
	e := &EmulatedPTY{
		inner:  inner,
		state:  newState(rows, cols),
		closed: make(chan struct{}),
	}
	e.input, e.output = io.Pipe()
	go e.readerLoop()
	return e
}

// newState initializes a clean screen grid.
func newState(rows, cols int) *TerminalState {
	screen := make([][]Cell, rows)
	for i := range rows {
		screen[i] = make([]Cell, cols)
		for j := range cols {
			screen[i][j] = Cell{Rune: ' '}
		}
	}
	return &TerminalState{Rows: rows, Cols: cols, Screen: screen}
}

// readerLoop continuously reads raw data from the PTY,
// concatenates broken UTF-8 characters, and sends them to the parser for processing.
func (e *EmulatedPTY) readerLoop() {
	buf := make([]byte, 8192)
	var remainder []byte // Stores the bytes of a UTF-8 character "broken" at a chunk boundary.

	for {
		n, err := e.inner.Read(buf)
		if n > 0 {
			data := append(remainder, buf[:n]...)

			// We are looking for the boundary of the last valid UTF-8 character in the entire chunk.
			lastFull := 0
			for i := 0; i < len(data); {
				r, size := utf8.DecodeRune(data[i:])
				// If a decoding error is encountered at the very end of the buffer,
				// it's possible that the character was simply not read to the end.
				if r == utf8.RuneError && size == 1 && len(data[i:]) < utf8.UTFMax {
					break
				}
				i += size
				lastFull = i
			}

			if lastFull > 0 {
				e.mu.Lock()
				e.processInput(data[:lastFull])
				e.mu.Unlock()
				// We save the "tail" for the next reading iteration.
				remainder = append([]byte(nil), data[lastFull:]...)
			} else {
				remainder = data
			}
		}

		// Handling handle closing or I/O errors
		if err != nil {
			if len(remainder) > 0 {
				e.mu.Lock()
				e.processInput(remainder)
				e.mu.Unlock()
			}
			e.err = err
			_ = e.output.CloseWithError(err)
			close(e.closed)
			return
		}
	}
}

// processInput is a state machine for parsing a byte stream.
// It strips out garbage OSC sequences and outputs valid data to the Pipe.
func (e *EmulatedPTY) processInput(p []byte) {
	for i := 0; i < len(p); {
		r, size := utf8.DecodeRune(p[i:])
		currentByte := p[i : i+size]
		i += size

		switch e.parserState {
		case stateNormal:
			if r == 0x1b { // ESC
				e.parserState = stateEscape
			} else {
				e.handleRune(r)
				// In Pipe, we write only plain text.
				_, _ = e.output.Write(currentByte)
			}

		case stateEscape:
			if r == '[' {
				e.parserState = stateCSI
				e.csiBuf = e.csiBuf[:0]
				// Pass ESC [ to Pipe to preserve colors in the output stream
				_, _ = e.output.Write([]byte{0x1b, '['})
			} else if r == ']' {
				e.parserState = stateOSC // Start of garbage window control command
			} else {
				e.parserState = stateNormal
			}

		case stateCSI:
			// CSI parameters (numbers, semicolons) are also sent to Pipe
			_, _ = e.output.Write(currentByte)
			if r >= 0x40 && r <= 0x7E { // The final character of the command (usually 'm', 'H', 'J')
				e.handleCSI(byte(r), string(e.csiBuf))
				e.parserState = stateNormal
			} else {
				e.csiBuf = append(e.csiBuf, currentByte...)
			}

		case stateOSC:
			// OSC (Operating System Command) - usually a window title or shell integration.
			// Ends with either BEL (\x07) or ST (ESC \).
			if r == 0x07 {
				e.parserState = stateNormal
			} else if r == 0x1b {
				// Encountered ESC - perhaps this is the beginning of ST (\x1b\)
				e.parserState = stateOSCEscape
			}
			// Everything else inside OSC is simply ignored (we don't write it to Pipe)

		case stateOSCEscape:
			// We got here by encountering ESC inside OSC.
			// If the current character is '\', then the ST sequence (\x1b\) is complete.
			if r == '\\' {
				e.parserState = stateNormal
			} else {
				// If any other character arrived, it was a "broken" OSC
				// or the beginning of a new ESC sequence.
				// To be safe, exit to Normal.
				e.parserState = stateNormal
			}
		}
	}
}

// handleRune processes a single printable character or basic escape sequence.
func (e *EmulatedPTY) handleRune(r rune) {
	s := e.state
	switch r {
	case '\r':
		s.CursorX = 0
	case '\n':
		s.CursorY++
	case '\t':
		s.CursorX = (s.CursorX/8 + 1) * 8
	case '\b':
		if s.CursorX > 0 {
			s.CursorX--
		}
	case 0x07: // Ignore BEL (beep) in the main thread
	default:
		// Checking whether scrolling is required before output
		if s.CursorY >= s.Rows {
			e.scroll()
		}
		// Automatic line wrap (Wrap-around)
		if s.CursorX >= s.Cols {
			s.CursorX = 0
			s.CursorY++
			if s.CursorY >= s.Rows {
				e.scroll()
			}
		}

		// Calculating the visual width of a character (important for emoji and special characters)
		w := runewidth.RuneWidth(r)
		if w <= 0 {
			return
		}

		// Write a symbol to the current cell with active styles
		s.Screen[s.CursorY][s.CursorX] = Cell{
			Rune: r,
			FG:   s.CurFG,
			BG:   s.CurBG,
		}
		s.CursorX += w
	}
}

// scroll выполняет классическую прокрутку терминала вверх на 1 строку.
func (e *EmulatedPTY) scroll() {
	s := e.state
	copy(s.Screen, s.Screen[1:]) // Move all lines up

	// Create a new empty line at the very bottom
	s.Screen[s.Rows-1] = make([]Cell, s.Cols)
	for j := range s.Cols {
		s.Screen[s.Rows-1][j] = Cell{Rune: ' '}
	}
	s.CursorY = s.Rows - 1
}

// handleCSI parses control sequences (colors, clear, cursor position).
func (e *EmulatedPTY) handleCSI(final byte, params string) {
	switch final {
	case 'm': // SGR (Select Graphic Rendition)
		if params == "" || params == "0" {
			e.state.CurFG, e.state.CurBG = "", ""
			return
		}
		parts := strings.Split(params, ";")
		for _, p := range parts {
			// Processing of basic colors (30-37, 90-97 — FG; 40-47, 100-107 — BG)
			if strings.HasPrefix(p, "3") || strings.HasPrefix(p, "9") {
				e.state.CurFG = "\x1b[" + p + "m"
			} else if strings.HasPrefix(p, "4") || strings.HasPrefix(p, "10") {
				e.state.CurBG = "\x1b[" + p + "m"
			}
		}
	case 'H', 'f': // CUP (Cursor Position) — moving the cursor
		var r, c int
		fmt.Sscanf(params, "%d;%d", &r, &c)
		e.state.CursorY = max(0, min(r-1, e.state.Rows-1))
		e.state.CursorX = max(0, min(c-1, e.state.Cols-1))
	case 'J': // ED (Erase in Display) – screen cleaning
		if params == "2" {
			// Parameter '2' means to clear the entire screen
			e.state = newState(e.state.Rows, e.state.Cols)
		}
	}
}

// RenderScreen generates a text screenshot of the current screen state
// preserving all ANSI colors for correct display in the terminal.
func (e *EmulatedPTY) RenderScreen() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]string, e.state.Rows)
	for i, row := range e.state.Screen {
		var line strings.Builder
		curFG, curBG := "", ""

		for _, cell := range row {
			// Optimization: Change colors in a row only when they actually change
			if cell.FG != curFG || cell.BG != curBG {
				line.WriteString("\x1b[0m") // Reset previous styles
				line.WriteString(cell.FG)
				line.WriteString(cell.BG)
				curFG, curBG = cell.FG, cell.BG
			}
			line.WriteRune(cell.Rune)
		}
		// If there is an active color left at the end of the line, reset it
		if curFG != "" || curBG != "" {
			line.WriteString("\x1b[0m")
		}
		out[i] = line.String()
	}
	return out
}

// Read reads filtered data (without OSC "garbage") from the emulator's internal buffer.
func (e *EmulatedPTY) Read(p []byte) (int, error) { return e.input.Read(p) }

// Write sends data directly to the underlying real PTY process.
func (e *EmulatedPTY) Write(p []byte) (int, error) { return e.inner.Write(p) }

// Close closes PTY handles and stops background processes.
func (e *EmulatedPTY) Close() error {
	_ = e.inner.Close()
	return nil
}

// Pid returns the process ID of the process running inside the PTY.
func (e *EmulatedPTY) Pid() int { return e.inner.Pid() }

// WaitContext waits for the process in the context-aware PTY to terminate.
func (e *EmulatedPTY) WaitContext(ctx context.Context) (uint32, error) {
	return e.inner.WaitContext(ctx)
}

// Resize notifies the system that the window size has changed and rebuilds the internal grid.
func (e *EmulatedPTY) Resize(cols, rows int) error {
	// First notify the process in the PTY to redraw its output (e.g. bash/zsh)
	if err := e.inner.Resize(cols, rows); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Create a new grid of the desired size, filled with spaces
	newScreen := make([][]Cell, rows)
	for i := range rows {
		newScreen[i] = make([]Cell, cols)
		for j := range cols {
			newScreen[i][j] = Cell{Rune: ' '}
		}
	}

	// Copy data from the old grid to the new one.
	// Define the intersection area to ensure that neither the old nor the new array goes beyond its boundaries.
	copyRows := rows
	if e.state.Rows < copyRows {
		copyRows = e.state.Rows
	}
	copyCols := cols
	if e.state.Cols < copyCols {
		copyCols = e.state.Cols
	}

	for i := 0; i < copyRows; i++ {
		for j := 0; j < copyCols; j++ {
			newScreen[i][j] = e.state.Screen[i][j]
		}
	}

	// Update the state
	e.state.Screen = newScreen
	e.state.Rows = rows
	e.state.Cols = cols

	// Adjust the cursor position.
	if e.state.CursorX >= cols {
		e.state.CursorX = cols - 1
	}
	if e.state.CursorY >= rows {
		e.state.CursorY = rows - 1
	}

	if e.state.CursorX < 0 {
		e.state.CursorX = 0
	}
	if e.state.CursorY < 0 {
		e.state.CursorY = 0
	}

	return nil
}
