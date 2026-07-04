// Package terminal implements the terminal screen model: a two-dimensional
// grid of Cells driven by a vte.Performer callback stream.
//
// Terminal is the central type. It wraps a primary and an alternate screen,
// a scrollback ring buffer, and the current SGR pen state. All public methods
// that read screen content are guarded by an RWMutex so the renderer can read
// concurrently while the PTY goroutine writes.
package terminal

import (
	"sync"
	"unicode/utf8"

	"github.com/termizard/termizard/internal/core/vte"
)

// Terminal implements vte.Performer and maintains the complete terminal state.
// Methods called from the PTY goroutine (Print, Execute, CSI, OSC, …) must
// hold the write lock; renderer reads hold the read lock.
type Terminal struct {
	mu sync.RWMutex

	primary *screen
	alt     *screen
	active  *screen // points to primary or alt

	scrollback *Scrollback

	// reflowSavedLines holds logical lines scrolled off the top during a width
	// reflow that did not fit in the row budget. Prepended on the next resize so
	// expanding the window can restore them without merging unrelated rows.
	reflowSavedLines [][]Cell
	// reflowGridContinues is true when the first visible row in the grid is the
	// tail of a logical line whose head was saved in reflowSavedLines.
	reflowGridContinues bool

	title   string
	onTitle func(string) // called on OSC 0/2 title change
	onBell  func()       // called on BEL

	// Terminal modes
	appCursorKeys  bool // DECCKM  ?1h  — application cursor-key sequences
	appKeypad      bool // DECKPAM/DECKPNM
	bracketedPaste bool // ?2004h
	autoWrap       bool // DECAWM  ?7h  (default on)

	reflowOnResize bool // when true, re-break wrapped lines on column resize
}

// compile-time check: Terminal implements vte.Performer.
var _ vte.Performer = (*Terminal)(nil)

// New creates a Terminal with the given initial size and scrollback capacity.
// reflowOnResize controls whether column resizes re-break soft-wrapped lines.
func New(cols, rows, scrollbackLines int, reflowOnResize bool) *Terminal {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	primary := newScreen(cols, rows)
	alt := newScreen(cols, rows)
	t := &Terminal{
		primary:        primary,
		alt:            alt,
		active:         primary,
		scrollback:     newScrollback(scrollbackLines),
		autoWrap:       true,
		reflowOnResize: reflowOnResize,
	}
	return t
}

// ── Public read API (renderer calls these under RLock) ──────────────────────

// RLock / RUnlock expose the read lock so the renderer can snapshot
// multiple fields atomically without going through an accessor per call.
func (t *Terminal) RLock()   { t.mu.RLock() }
func (t *Terminal) RUnlock() { t.mu.RUnlock() }

func (t *Terminal) Cols() int { return t.active.cols() }
func (t *Terminal) Rows() int { return t.active.rows() }

// Cell returns the cell at (row, col) in the active screen.
func (t *Terminal) Cell(row, col int) Cell { return t.active.grid.Cell(row, col) }

// IsDirty reports whether row has changed since the last ClearDirty call.
func (t *Terminal) IsDirty(row int) bool { return t.active.grid.IsDirty(row) }

// ClearDirty resets all dirty flags. Call after each rendered frame.
func (t *Terminal) ClearDirty() { t.active.grid.ClearDirty() }

// CursorPos returns the cursor's (col, row) in the active screen.
func (t *Terminal) CursorPos() (col, row int) {
	return t.active.cursor.col, t.active.cursor.row
}

// CursorVisible reports whether the cursor should be drawn.
func (t *Terminal) CursorVisible() bool { return t.active.cursor.visible }

// Title returns the last window title set via OSC 0/2.
func (t *Terminal) Title() string { return t.title }

// AppCursorKeys reports whether DECCKM application cursor-key mode is active.
// The frontend uses this to choose between \x1b[A and \x1bOA for arrow keys.
func (t *Terminal) AppCursorKeys() bool { return t.appCursorKeys }

// BracketedPaste reports whether bracketed paste mode is active.
func (t *Terminal) BracketedPaste() bool { return t.bracketedPaste }

// SetOnTitle registers a callback for OSC title changes. Safe to call before
// any PTY data arrives.
func (t *Terminal) SetOnTitle(fn func(string)) {
	t.mu.Lock()
	t.onTitle = fn
	t.mu.Unlock()
}

// SetOnBell registers a callback for BEL (0x07).
func (t *Terminal) SetOnBell(fn func()) {
	t.mu.Lock()
	t.onBell = fn
	t.mu.Unlock()
}

// Resize adjusts both screens and moves the cursor into bounds.
// When the primary screen shrinks in height, lines that would hide the cursor
// are pushed into the scrollback buffer so they remain accessible.
func (t *Terminal) Resize(cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if t.primary.cols() == cols && t.primary.rows() == rows {
		return
	}

	if !t.reflowOnResize {
		t.reflowSavedLines = nil
		t.reflowGridContinues = false
	}

	onAlt := t.active == t.alt

	primary, saved, continues := t.primary.resize(cols, rows, t.reflowOnResize, t.reflowSavedLines, t.reflowGridContinues)
	t.reflowSavedLines = saved
	t.reflowGridContinues = continues
	t.primary = primary

	t.alt, _, _ = t.alt.resize(cols, rows, t.reflowOnResize, nil, false)

	if onAlt {
		t.active = t.alt
	} else {
		t.active = t.primary
	}
}

// ScrollbackLine returns the line at scrollback index idx (0 = most recent).
func (t *Terminal) ScrollbackLine(idx int) []Cell { return t.scrollback.Line(idx) }

// ScrollbackLen returns the number of lines in the scrollback buffer.
func (t *Terminal) ScrollbackLen() int { return t.scrollback.Len() }

// ── vte.Performer ────────────────────────────────────────────────────────────

// Print is called for every printable rune (including wide Unicode).
func (t *Terminal) Print(r rune) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s := t.active
	w := runeWidth(r)

	// Pending-wrap: cursor was at the last column; wrap now before placing char.
	if s.pendingWrap && t.autoWrap {
		// Mark the row we're about to leave so reflow can rejoin it later.
		s.grid.markWrapped(s.cursor.row)
		s.cursor.col = 0
		t.newline(false) // CR+LF semantics for wrap
	}
	s.pendingWrap = false

	// If wide char doesn't fit in the last column, pad with space and wrap.
	if w == 2 && s.cursor.col == s.cols()-1 {
		s.grid.setCell(s.cursor.row, s.cursor.col, s.pen(' ', 1))
		if t.autoWrap {
			s.grid.markWrapped(s.cursor.row)
			s.cursor.col = 0
			t.newline(false)
		}
	}

	s.grid.setCell(s.cursor.row, s.cursor.col, s.pen(r, uint8(w)))
	if w == 2 && s.cursor.col+1 < s.cols() {
		// Wide chars occupy two columns; mark the continuation cell.
		placeholder := s.pen(0, 1) // NUL rune = placeholder for wide right half
		s.grid.setCell(s.cursor.row, s.cursor.col+1, placeholder)
	}

	s.cursor.col += w
	if s.cursor.col >= s.cols() {
		s.cursor.col = s.cols() - 1
		s.pendingWrap = true
	}
}

// Execute handles C0/C1 control bytes.
func (t *Terminal) Execute(b byte) {
	t.mu.Lock()
	s := t.active
	var bellCb func()
	switch b {
	case 0x07: // BEL
		bellCb = t.onBell
	case 0x08: // BS
		if s.cursor.col > 0 {
			s.cursor.col--
		}
		s.pendingWrap = false
	case 0x09: // HT (horizontal tab)
		next := (s.cursor.col/8 + 1) * 8
		if next >= s.cols() {
			next = s.cols() - 1
		}
		s.cursor.col = next
		s.pendingWrap = false
	case 0x0A, 0x0B, 0x0C: // LF, VT, FF
		t.newline(false)
		s.pendingWrap = false
	case 0x0D: // CR
		s.cursor.col = 0
		s.pendingWrap = false
	case 0x0E: // SO — charset G1
	case 0x0F: // SI — charset G0
	}
	t.mu.Unlock()
	if bellCb != nil {
		bellCb()
	}
}

// EscDispatch handles two-character ESC sequences.
func (t *Terminal) EscDispatch(intermediates []byte, final byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s := t.active
	if len(intermediates) == 0 {
		switch final {
		case '7': // DECSC save cursor
			s.saveCursor()
		case '8': // DECRC restore cursor
			s.restoreCursor()
		case 'M': // RI reverse index (scroll down if at top of scroll region)
			if s.cursor.row == s.scrollTop {
				s.grid.scrollDown(s.scrollTop, s.scrollBot, 1)
			} else if s.cursor.row > 0 {
				s.cursor.row--
			}
			s.pendingWrap = false
		case 'c': // RIS full reset
			t.fullReset()
		}
	}
}

// CSI handles CSI escape sequences.
func (t *Terminal) CSI(params [][]uint16, intermediates []byte, ignore bool, final rune) {
	if ignore {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	s := t.active
	// Private mode prefix '?'
	private := len(intermediates) == 1 && intermediates[0] == '?'

	switch final {
	case 'm': // SGR — select graphic rendition
		applySGR(s, params)

	case 'H', 'f': // CUP — cursor position
		row := int(p1(params, 0, 1)) - 1
		col := int(p1(params, 1, 1)) - 1
		if row < 0 {
			row = 0
		}
		if col < 0 {
			col = 0
		}
		if row >= s.rows() {
			row = s.rows() - 1
		}
		if col >= s.cols() {
			col = s.cols() - 1
		}
		s.cursor.row, s.cursor.col = row, col
		s.pendingWrap = false

	case 'A': // CUU — cursor up
		n := max1(params)
		s.cursor.row -= n
		if s.cursor.row < 0 {
			s.cursor.row = 0
		}
		s.pendingWrap = false

	case 'B': // CUD — cursor down
		n := max1(params)
		s.cursor.row += n
		if s.cursor.row >= s.rows() {
			s.cursor.row = s.rows() - 1
		}
		s.pendingWrap = false

	case 'C': // CUF — cursor forward (right)
		n := max1(params)
		s.cursor.col += n
		if s.cursor.col >= s.cols() {
			s.cursor.col = s.cols() - 1
		}
		s.pendingWrap = false

	case 'D': // CUB — cursor back (left)
		n := max1(params)
		s.cursor.col -= n
		if s.cursor.col < 0 {
			s.cursor.col = 0
		}
		s.pendingWrap = false

	case 'E': // CNL — cursor next line
		n := max1(params)
		s.cursor.row += n
		if s.cursor.row >= s.rows() {
			s.cursor.row = s.rows() - 1
		}
		s.cursor.col = 0
		s.pendingWrap = false

	case 'F': // CPL — cursor previous line
		n := max1(params)
		s.cursor.row -= n
		if s.cursor.row < 0 {
			s.cursor.row = 0
		}
		s.cursor.col = 0
		s.pendingWrap = false

	case 'G': // CHA — cursor horizontal absolute
		col := int(p1(params, 0, 1)) - 1
		if col < 0 {
			col = 0
		}
		if col >= s.cols() {
			col = s.cols() - 1
		}
		s.cursor.col = col
		s.pendingWrap = false

	case 'd': // VPA — vertical position absolute
		row := int(p1(params, 0, 1)) - 1
		if row < 0 {
			row = 0
		}
		if row >= s.rows() {
			row = s.rows() - 1
		}
		s.cursor.row = row
		s.pendingWrap = false

	case 'J': // ED — erase in display
		n := int(p1(params, 0, 0))
		t.eraseDisplay(s, n)

	case 'K': // EL — erase in line
		n := int(p1(params, 0, 0))
		t.eraseLine(s, n)

	case 'X': // ECH — erase characters (no cursor movement)
		n := max1(params)
		end := s.cursor.col + n - 1
		if end >= s.cols() {
			end = s.cols() - 1
		}
		s.grid.clearLine(s.cursor.row, s.cursor.col, end)

	case 'L': // IL — insert lines
		n := max1(params)
		s.grid.scrollDown(s.cursor.row, s.scrollBot, n)

	case 'M': // DL — delete lines
		n := max1(params)
		s.grid.scrollUp(s.cursor.row, s.scrollBot, n)

	case 'P': // DCH — delete characters
		n := max1(params)
		row := s.cursor.row
		col := s.cursor.col
		end := s.cols()
		// Shift cells left
		for c := col; c < end-n; c++ {
			s.grid.setCell(row, c, s.grid.Cell(row, c+n))
		}
		for c := end - n; c < end; c++ {
			s.grid.setCell(row, c, blankCell)
		}

	case '@': // ICH — insert blank characters
		n := max1(params)
		row := s.cursor.row
		col := s.cursor.col
		end := s.cols()
		// Shift cells right, dropping those that fall off the edge
		for c := end - 1; c >= col+n; c-- {
			s.grid.setCell(row, c, s.grid.Cell(row, c-n))
		}
		for c := col; c < col+n && c < end; c++ {
			s.grid.setCell(row, c, blankCell)
		}

	case 'S': // SU — scroll up
		n := max1(params)
		for i := 0; i < n; i++ {
			t.scrollUpOne(s)
		}

	case 'T': // SD — scroll down
		n := max1(params)
		s.grid.scrollDown(s.scrollTop, s.scrollBot, n)

	case 'r': // DECSTBM — set scrolling region
		top := int(p1(params, 0, 1)) - 1
		bot := int(p1(params, 1, uint16(s.rows()))) - 1
		if top < 0 {
			top = 0
		}
		if bot >= s.rows() {
			bot = s.rows() - 1
		}
		if top < bot {
			s.scrollTop = top
			s.scrollBot = bot
		}
		s.cursor.row, s.cursor.col = 0, 0
		s.pendingWrap = false

	case 's': // DECSC (some terminals use CSI s)
		if !private {
			s.saveCursor()
		}

	case 'u': // DECRC (some terminals use CSI u)
		if !private {
			s.restoreCursor()
		}

	case 'n': // DSR — device status report
		// 6 → report cursor position; handled by writing to PTY (not done here)

	case 'h': // SM — set mode
		t.setMode(s, params, private, true)

	case 'l': // RM — reset mode
		t.setMode(s, params, private, false)
	}
}

// OSC handles Operating System Command strings.
func (t *Terminal) OSC(params [][]byte, _ bool) {
	if len(params) == 0 {
		return
	}
	var onTitle func(string)
	var newTitle string

	t.mu.Lock()
	switch string(params[0]) {
	case "0", "1", "2": // set icon name and/or window title
		if len(params) >= 2 {
			newTitle = string(params[1])
			t.title = newTitle
			onTitle = t.onTitle
		}
	}
	t.mu.Unlock()

	if onTitle != nil {
		onTitle(newTitle)
	}
}

// DCS / DCSPut / DCSUnhook — not used in MVP; satisfy the interface.
func (t *Terminal) DCS(_ [][]uint16, _ []byte, _ bool, _ rune) {}
func (t *Terminal) DCSPut(_ byte)                              {}
func (t *Terminal) DCSUnhook()                                 {}

// ── Internal helpers ─────────────────────────────────────────────────────────

// newline performs a linefeed. If withCR is true, it also resets the column
// (used for CRLF mode). Callers hold the write lock.
func (t *Terminal) newline(withCR bool) {
	s := t.active
	if withCR {
		s.cursor.col = 0
	}
	if s.cursor.row == s.scrollBot {
		t.scrollUpOne(s)
	} else if s.cursor.row < s.rows()-1 {
		s.cursor.row++
	}
}

// scrollUpOne scrolls the scroll region up by 1, pushing the top line into
// the scrollback buffer (primary screen only).
func (t *Terminal) scrollUpOne(s *screen) {
	if s == t.primary {
		t.scrollback.Push(s.grid.lineForScrollback(s.scrollTop))
	}
	s.grid.scrollUp(s.scrollTop, s.scrollBot, 1)
}

// eraseDisplay clears part of the active screen.
// mode: 0=below cursor, 1=above cursor, 2=all, 3=all+scrollback.
func (t *Terminal) eraseDisplay(s *screen, mode int) {
	row, col := s.cursor.row, s.cursor.col
	switch mode {
	case 0: // erase from cursor to end of screen
		s.grid.clearLine(row, col, s.cols()-1)
		for r := row + 1; r < s.rows(); r++ {
			s.grid.clearLine(r, 0, s.cols()-1)
		}
	case 1: // erase from start of screen to cursor
		for r := 0; r < row; r++ {
			s.grid.clearLine(r, 0, s.cols()-1)
		}
		s.grid.clearLine(row, 0, col)
	case 2: // erase entire screen
		for r := 0; r < s.rows(); r++ {
			s.grid.clearLine(r, 0, s.cols()-1)
		}
	case 3: // erase screen + scrollback (xterm extension)
		for r := 0; r < s.rows(); r++ {
			s.grid.clearLine(r, 0, s.cols()-1)
		}
		t.scrollback = newScrollback(t.scrollback.cap)
	}
}

// eraseLine clears part of the current line.
// mode: 0=to end, 1=to start, 2=entire line.
func (t *Terminal) eraseLine(s *screen, mode int) {
	row, col := s.cursor.row, s.cursor.col
	switch mode {
	case 0:
		s.grid.clearLine(row, col, s.cols()-1)
	case 1:
		s.grid.clearLine(row, 0, col)
	case 2:
		s.grid.clearLine(row, 0, s.cols()-1)
	}
}

// setMode handles CSI h/l (SM/RM) for private (?) and standard modes.
func (t *Terminal) setMode(s *screen, params [][]uint16, private, on bool) {
	for _, p := range params {
		v := paramVal(p)
		if private {
			switch v {
			case 1: // DECCKM — application cursor keys
				t.appCursorKeys = on
			case 7: // DECAWM — auto-wrap mode
				t.autoWrap = on
			case 12: // cursor blink (ignore in model; frontend handles it)
			case 25: // cursor visibility
				s.cursor.visible = on
			case 1049: // alt screen + save/restore cursor
				if on {
					t.primary.saveCursor()
					t.active = t.alt
					t.eraseDisplay(t.alt, 2)
					t.alt.cursor.row, t.alt.cursor.col = 0, 0
					t.alt.pendingWrap = false
				} else {
					t.active = t.primary
					t.primary.restoreCursor()
				}
			case 47, 1047: // alt screen without save/restore
				if on {
					t.active = t.alt
					t.eraseDisplay(t.alt, 2)
				} else {
					t.active = t.primary
				}
			case 2004: // bracketed paste
				t.bracketedPaste = on
			}
		}
		// Standard modes (not private) handled as needed
	}
}

// fullReset clears all state (ESC c — RIS).
func (t *Terminal) fullReset() {
	cols, rows := t.active.cols(), t.active.rows()
	t.primary = newScreen(cols, rows)
	t.alt = newScreen(cols, rows)
	t.active = t.primary
	t.appCursorKeys = false
	t.appKeypad = false
	t.bracketedPaste = false
	t.autoWrap = true
}

// ── Parameter helpers ────────────────────────────────────────────────────────

// p1 returns the idx-th parameter value, defaulting to def if missing or zero.
func p1(params [][]uint16, idx int, def uint16) uint16 {
	if idx >= len(params) {
		return def
	}
	v := paramVal(params[idx])
	if v == 0 {
		return def
	}
	return v
}

// max1 returns the first CSI parameter as an int, minimum 1.
func max1(params [][]uint16) int {
	v := int(p1(params, 0, 1))
	if v < 1 {
		return 1
	}
	return v
}

// ── Unicode width ────────────────────────────────────────────────────────────

// runeWidth returns 2 for wide (CJK / emoji) runes, 1 for everything else.
// Based on Unicode East Asian Width property; covers the common ranges.
func runeWidth(r rune) int {
	if r < 0x1100 {
		return 1
	}
	switch {
	// Hangul Jamo
	case r <= 0x115F:
		return 2
	// CJK Radicals Supplement … CJK Unified Ideographs Extension A
	case r >= 0x2E80 && r <= 0x3247:
		return 2
	// CJK Unified Ideographs Extension A (partial)
	case r >= 0x3250 && r <= 0x4DBF:
		return 2
	// CJK Unified Ideographs
	case r >= 0x4E00 && r <= 0xA4C6:
		return 2
	// Hangul Jamo Extended-A
	case r >= 0xA960 && r <= 0xA97C:
		return 2
	// Hangul Syllables
	case r >= 0xAC00 && r <= 0xD7A3:
		return 2
	// CJK Compatibility Ideographs
	case r >= 0xF900 && r <= 0xFAFF:
		return 2
	// Vertical Forms, CJK Compatibility Forms, Enclosed CJK
	case r >= 0xFE10 && r <= 0xFE19:
		return 2
	case r >= 0xFE30 && r <= 0xFE6B:
		return 2
	// Fullwidth Latin
	case r >= 0xFF01 && r <= 0xFF60:
		return 2
	case r >= 0xFFE0 && r <= 0xFFE6:
		return 2
	// CJK Unified Ideographs Extension B+
	case r >= 0x1B000 && r <= 0x1B001:
		return 2
	case r >= 0x1F004 && r <= 0x1F0CF:
		return 2
	case r >= 0x1F200 && r <= 0x1F251:
		return 2
	// Emoji: Emoticons, Misc Symbols and Pictographs, Transport, etc.
	case r >= 0x1F300 && r <= 0x1F9FF:
		return 2
	case r >= 0x20000 && r <= 0x2FFFD:
		return 2
	case r >= 0x30000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

// Ensure unicode/utf8 is used (keeps the import if we add UTF-8 helpers later).
var _ = utf8.RuneLen
