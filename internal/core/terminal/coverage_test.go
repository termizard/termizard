package terminal_test

import (
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/core/terminal"
)

func TestTerminalModesAndCallbacks(t *testing.T) {
	term := terminal.New(10, 5, 100, true)

	bell := 0
	term.SetOnBell(func() { bell++ })
	feed(t, term, "\x07")
	if bell != 1 {
		t.Fatalf("BEL callback count = %d, want 1", bell)
	}

	feed(t, term, "\x1b[?1h\x1b[?2004h\x1b[?25l")
	if !term.AppCursorKeys() {
		t.Fatal("expected application cursor keys enabled")
	}
	if !term.BracketedPaste() {
		t.Fatal("expected bracketed paste enabled")
	}
	if term.CursorVisible() {
		t.Fatal("expected cursor hidden")
	}

	feed(t, term, "\x1b[?25h")
	if !term.CursorVisible() {
		t.Fatal("expected cursor visible after DECSET 25")
	}
}

func TestEscDispatchSaveRestoreAndReset(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "A")
	feed(t, term, "\x1b[C") // cursor to col 1
	feed(t, term, "\x1b7")  // DECSC
	feed(t, term, "\x1b[1;1H")
	feed(t, term, "\x1b8") // DECRC

	col, row := cursorAt(t, term)
	if col != 2 || row != 0 {
		t.Fatalf("after DECRC cursor = (%d,%d), want (2,0)", col, row)
	}

	feed(t, term, "\x1bc") // RIS
	if term.AppCursorKeys() {
		t.Fatal("expected application cursor keys cleared after RIS")
	}
	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("after RIS col0 = %q, want blank", c.Char)
	}
}

func TestExecuteControlBytes(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "ABCDE")
	feed(t, term, "\x08\x09") // BS then HT to next tab stop

	col, _ := cursorAt(t, term)
	if col != 8 {
		t.Fatalf("after BS+HT cursor col = %d, want 8", col)
	}
}

func TestResizeWithoutReflow(t *testing.T) {
	term := terminal.New(80, 10, 100, false)
	feed(t, term, strings.Repeat("X", 80))

	term.Resize(40, 10)
	if term.Cols() != 40 {
		t.Fatalf("Cols() = %d, want 40", term.Cols())
	}

	term.Resize(80, 10)
	if term.Cols() != 80 {
		t.Fatalf("Cols() = %d, want 80", term.Cols())
	}
}

func TestNewClampsInvalidSize(t *testing.T) {
	term := terminal.New(0, 0, 100, true)
	if term.Cols() != 80 || term.Rows() != 24 {
		t.Fatalf("New(0,0) size = (%d,%d), want (80,24)", term.Cols(), term.Rows())
	}
}

func TestWideRuneWidth(t *testing.T) {
	term := newTerm(10, 3)
	feed(t, term, "A\u4e16B") // CJK ideograph is wide

	if c := cellAt(t, term, 0, 0); c.Char != 'A' {
		t.Fatalf("col0 = %q, want A", c.Char)
	}
	if c := cellAt(t, term, 0, 1); c.Char != '\u4e16' {
		t.Fatalf("col1 = %q, want CJK", c.Char)
	}
	if c := cellAt(t, term, 0, 2); c.Char != 0 {
		t.Fatalf("col2 should be wide-char placeholder, got %q", c.Char)
	}
	if c := cellAt(t, term, 0, 3); c.Char != 'B' {
		t.Fatalf("col3 = %q, want B", c.Char)
	}
}

func TestEraseLineModes(t *testing.T) {
	term := newTerm(10, 3)
	feed(t, term, "HELLO")
	feed(t, term, "\x1b[1;3H") // cursor on H at col 2
	feed(t, term, "\x1b[1K")   // EL 1: erase from start to cursor

	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("EL1 col0 = %q, want space", c.Char)
	}
	if c := cellAt(t, term, 0, 4); c.Char != 'O' {
		t.Fatalf("EL1 col4 = %q, want O", c.Char)
	}
}

func TestEraseDisplayModes(t *testing.T) {
	term := newTerm(5, 3)
	feed(t, term, "AAAAA\r\nBBBBB\r\nCCCCC")
	feed(t, term, "\x1b[2;2H") // row 1 col 1
	feed(t, term, "\x1b[1J")   // ED 1: erase above

	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("ED1 row0 = %q, want blank", c.Char)
	}
	if c := cellAt(t, term, 2, 0); c.Char != 'C' {
		t.Fatalf("ED1 row2 = %q, want C", c.Char)
	}

	feed(t, term, "\x1b[2J") // ED 2: erase all
	for row := 0; row < 3; row++ {
		for col := 0; col < 5; col++ {
			if c := cellAt(t, term, row, col); c.Char != ' ' {
				t.Fatalf("ED2 (%d,%d) = %q, want blank", row, col, c.Char)
			}
		}
	}
}

func TestReverseIndex(t *testing.T) {
	term := newTerm(5, 3)
	feed(t, term, "AAAAA\r\nBBBBB\r\nCCCCC")
	feed(t, term, "\x1b[1;1H") // top of screen
	feed(t, term, "\x1bM")     // RI

	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("after RI row0 = %q, want blank", c.Char)
	}
	if c := cellAt(t, term, 1, 0); c.Char != 'A' {
		t.Fatalf("after RI row1 = %q, want A", c.Char)
	}
}

func TestAdditionalSGRAttributes(t *testing.T) {
	term := newTerm(10, 3)
	feed(t, term, "\x1b[2;3;4;7;9mZ\x1b[0m")
	c := cellAt(t, term, 0, 0)
	if c.Char != 'Z' {
		t.Fatalf("char = %q, want Z", c.Char)
	}
	if c.Attrs&(terminal.AttrDim|terminal.AttrItalic|terminal.AttrUnderline|terminal.AttrInverse|terminal.AttrStrikethrough) == 0 {
		t.Fatal("expected combined SGR attributes")
	}
}

func TestWideRuneWidthRanges(t *testing.T) {
	term := newTerm(20, 3)
	// Hangul syllable + emoji should both consume two columns.
	feed(t, term, "A\uac00\U0001F600B")
	if cellAt(t, term, 0, 0).Char != 'A' {
		t.Fatal("expected A at col0")
	}
	if cellAt(t, term, 0, 5).Char != 'B' {
		t.Fatalf("expected B after wide chars, got %q at col5", cellAt(t, term, 0, 5).Char)
	}
}

func TestAltScreenWithoutSave(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "PRIMARY")
	feed(t, term, "\x1b[?1047h")
	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("alt screen should be blank, got %q", c.Char)
	}
	feed(t, term, "ALT")
	feed(t, term, "\x1b[?1047l")
	if c := cellAt(t, term, 0, 0); c.Char != 'P' {
		t.Fatalf("primary screen = %q, want P", c.Char)
	}
}

func TestAutoWrapDisable(t *testing.T) {
	term := newTerm(5, 3)
	feed(t, term, "\x1b[?7l")
	feed(t, term, strings.Repeat("X", 10))
	if c := cellAt(t, term, 0, 4); c.Char != 'X' {
		t.Fatalf("last col = %q, want X", c.Char)
	}
}

func TestSetOnTitleCallback(t *testing.T) {
	term := newTerm(10, 5)
	got := ""
	term.SetOnTitle(func(title string) { got = title })
	feed(t, term, "\x1b]0;Hello\x07")
	if got != "Hello" {
		t.Fatalf("title callback = %q, want Hello", got)
	}
	if term.Title() != "Hello" {
		t.Fatalf("Title() = %q, want Hello", term.Title())
	}
}

func TestExecuteCharsetSelect(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "\x0e\x0f") // SO/SI — no visible change expected
	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("unexpected char %q after charset select", c.Char)
	}
}

func TestSGRTrueColorAndBold(t *testing.T) {
	term := newTerm(10, 3)
	feed(t, term, "\x1b[1;38;2;255;128;64mX\x1b[0m")
	c := cellAt(t, term, 0, 0)
	if c.Char != 'X' {
		t.Fatalf("char = %q, want X", c.Char)
	}
	if c.Attrs&terminal.AttrBold == 0 {
		t.Fatal("expected bold attribute")
	}
}
