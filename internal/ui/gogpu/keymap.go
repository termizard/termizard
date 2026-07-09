package gogpu

import "github.com/gogpu/gpucontext"

// keyToSeq converts a key press event to the byte sequence written to the PTY.
// Returns nil for keys that produce no output (modifier-only, unknown, etc.).
// Printable characters are handled via SetOnTextInput, not here.
func keyToSeq(key gpucontext.Key, mods gpucontext.Modifiers, appCursor bool) []byte {
	// Ctrl+letter → C0 control bytes (Ctrl+A=0x01 … Ctrl+Z=0x1A).
	if mods.HasControl() && !mods.HasShift() {
		if b, ok := keyToCtrl(key); ok {
			if mods.HasAlt() {
				return []byte{0x1b, b}
			}
			return []byte{b}
		}
	}

	// Application cursor mode swaps arrow/home/end escape sequences.
	if appCursor {
		if seq, ok := appCursorSeqs[key]; ok {
			return withAlt(mods, seq)
		}
	}

	if seq, ok := specialSeqs[key]; ok {
		return withAlt(mods, seq)
	}
	return nil
}

// withAlt prepends ESC when Alt is held, matching xterm behavior.
func withAlt(mods gpucontext.Modifiers, seq string) []byte {
	if mods.HasAlt() {
		return append([]byte{0x1b}, []byte(seq)...)
	}
	return []byte(seq)
}

// keyToCtrl maps a key + Ctrl to a C0 control byte.
func keyToCtrl(key gpucontext.Key) (byte, bool) {
	// Letters: Ctrl+A=0x01 … Ctrl+Z=0x1A
	if key >= gpucontext.KeyA && key <= gpucontext.KeyZ {
		return byte(key-gpucontext.KeyA) + 1, true
	}
	switch key {
	case gpucontext.KeyBackspace:
		return 0x08, true
	case gpucontext.KeyEnter:
		return 0x0D, true
	case gpucontext.KeySpace:
		return 0x00, true
	case gpucontext.KeyBackslash:
		return 0x1C, true
	}
	return 0, false
}

// Escape sequences that appear in both specialSeqs and tests.
const (
	seqCursorUp = "\x1b[A"
	seqF1Str    = "\x1bOP"
)

// specialSeqs maps non-printable keys to their ANSI/xterm escape sequences.
var specialSeqs = map[gpucontext.Key]string{
	gpucontext.KeyUp:        seqCursorUp,
	gpucontext.KeyDown:      "\x1b[B",
	gpucontext.KeyRight:     "\x1b[C",
	gpucontext.KeyLeft:      "\x1b[D",
	gpucontext.KeyHome:      "\x1b[H",
	gpucontext.KeyEnd:       "\x1b[F",
	gpucontext.KeyPageUp:    "\x1b[5~",
	gpucontext.KeyPageDown:  "\x1b[6~",
	gpucontext.KeyInsert:    "\x1b[2~",
	gpucontext.KeyDelete:    "\x1b[3~",
	gpucontext.KeyF1:        seqF1Str,
	gpucontext.KeyF2:        "\x1bOQ",
	gpucontext.KeyF3:        "\x1bOR",
	gpucontext.KeyF4:        "\x1bOS",
	gpucontext.KeyF5:        "\x1b[15~",
	gpucontext.KeyF6:        "\x1b[17~",
	gpucontext.KeyF7:        "\x1b[18~",
	gpucontext.KeyF8:        "\x1b[19~",
	gpucontext.KeyF9:        "\x1b[20~",
	gpucontext.KeyF10:       "\x1b[21~",
	gpucontext.KeyF11:       "\x1b[23~",
	gpucontext.KeyF12:       "\x1b[24~",
	gpucontext.KeyEscape:    "\x1b",
	gpucontext.KeyBackspace: "\x7f",
	gpucontext.KeyTab:       "\t",
	gpucontext.KeyEnter:     "\r",
}

// appCursorSeqs replaces arrow and home/end sequences when DECCKM (?1h) is active.
var appCursorSeqs = map[gpucontext.Key]string{
	gpucontext.KeyUp:    "\x1bOA",
	gpucontext.KeyDown:  "\x1bOB",
	gpucontext.KeyRight: "\x1bOC",
	gpucontext.KeyLeft:  "\x1bOD",
	gpucontext.KeyHome:  "\x1bOH",
	gpucontext.KeyEnd:   "\x1bOF",
}
