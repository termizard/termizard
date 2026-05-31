package terminal_test

import (
	"testing"

	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/core/vte"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newTerm(cols, rows int) *terminal.Terminal {
	return terminal.New(cols, rows, 1000)
}

// feed parses raw bytes through the VTE state machine into t.
func feed(t *testing.T, term *terminal.Terminal, data string) {
	t.Helper()
	p := vte.New()
	p.Advance(term, []byte(data))
}

// cellAt returns the cell at (row,col) from the terminal.
func cellAt(t *testing.T, term *terminal.Terminal, row, col int) terminal.Cell {
	t.Helper()
	term.RLock()
	defer term.RUnlock()
	return term.Cell(row, col)
}

func cursorAt(t *testing.T, term *terminal.Terminal) (col, row int) {
	t.Helper()
	term.RLock()
	defer term.RUnlock()
	return term.CursorPos()
}

// ── Print / cursor movement ───────────────────────────────────────────────────

func TestPrintMovesRight(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "AB")
	col, row := cursorAt(t, term)
	if col != 2 || row != 0 {
		t.Fatalf("cursor want (2,0) got (%d,%d)", col, row)
	}
	if c := cellAt(t, term, 0, 0); c.Char != 'A' {
		t.Fatalf("cell(0,0) want 'A' got %q", c.Char)
	}
	if c := cellAt(t, term, 0, 1); c.Char != 'B' {
		t.Fatalf("cell(0,1) want 'B' got %q", c.Char)
	}
}

func TestCR_LF(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "Hello\r\nWorld")
	if c := cellAt(t, term, 1, 0); c.Char != 'W' {
		t.Fatalf("want 'W' at (1,0) got %q", c.Char)
	}
}

func TestAutoWrap(t *testing.T) {
	term := newTerm(5, 5)
	feed(t, term, "ABCDE") // fills row 0, cursor should be at col 4 with pendingWrap
	feed(t, term, "F")     // should wrap to row 1, col 0
	col, row := cursorAt(t, term)
	if row != 1 || col != 1 {
		t.Fatalf("after wrap want cursor (1,1) got (%d,%d)", col, row)
	}
	if c := cellAt(t, term, 1, 0); c.Char != 'F' {
		t.Fatalf("want 'F' at (1,0) got %q", c.Char)
	}
}

// ── SGR ───────────────────────────────────────────────────────────────────────

func TestSGRBoldColor(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[1;32mX\x1b[0m")
	c := cellAt(t, term, 0, 0)
	if c.Attrs&terminal.AttrBold == 0 {
		t.Fatal("expected AttrBold")
	}
	if c.FG.Kind != terminal.ColorANSI || c.FG.Value != 2 {
		t.Fatalf("expected ANSI green fg, got %+v", c.FG)
	}
	// After reset: next char should have no attrs
	col, _ := cursorAt(t, term)
	feed(t, term, "Y")
	y := cellAt(t, term, 0, col)
	if y.Attrs != 0 {
		t.Fatalf("after reset attrs want 0 got %d", y.Attrs)
	}
}

func TestSGRTrueColor(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[38;2;255;128;0mA")
	c := cellAt(t, term, 0, 0)
	if c.FG.Kind != terminal.ColorRGB {
		t.Fatalf("expected ColorRGB, got %v", c.FG.Kind)
	}
	if c.FG.Value != (255<<16|128<<8|0) {
		t.Fatalf("expected RGB(255,128,0), got 0x%06X", c.FG.Value)
	}
}

func TestSGR256Color(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[38;5;200mA")
	c := cellAt(t, term, 0, 0)
	if c.FG.Kind != terminal.ColorIndexed || c.FG.Value != 200 {
		t.Fatalf("expected Indexed(200), got %+v", c.FG)
	}
}

func TestSGRBrightColors(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[93mA") // bright yellow fg
	c := cellAt(t, term, 0, 0)
	if c.FG.Kind != terminal.ColorANSI || c.FG.Value != 11 {
		t.Fatalf("expected ANSI(11), got %+v", c.FG)
	}
}

func TestSGRAllAttrs(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[1;2;3;4;5;7;8;9mA")
	c := cellAt(t, term, 0, 0)
	expected := terminal.AttrBold | terminal.AttrDim | terminal.AttrItalic |
		terminal.AttrUnderline | terminal.AttrBlink | terminal.AttrInverse |
		terminal.AttrInvisible | terminal.AttrStrikethrough
	if c.Attrs != expected {
		t.Fatalf("attrs want 0x%04X got 0x%04X", expected, c.Attrs)
	}
}

// ── Cursor positioning ────────────────────────────────────────────────────────

func TestCUP(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[5;10H") // row=5, col=10 (1-based)
	col, row := cursorAt(t, term)
	if col != 9 || row != 4 {
		t.Fatalf("CUP: want (col=9,row=4) got (%d,%d)", col, row)
	}
}

func TestCUP_Clamps(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "\x1b[99;99H") // out of bounds
	col, row := cursorAt(t, term)
	if col != 9 || row != 4 {
		t.Fatalf("CUP out-of-bounds: want (9,4) got (%d,%d)", col, row)
	}
}

func TestCursorMovement(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[10;10H") // row=9, col=9
	feed(t, term, "\x1b[2A")     // up 2 → row=7
	feed(t, term, "\x1b[3C")     // right 3 → col=12
	feed(t, term, "\x1b[1B")     // down 1 → row=8
	feed(t, term, "\x1b[4D")     // left 4 → col=8
	col, row := cursorAt(t, term)
	if col != 8 || row != 8 {
		t.Fatalf("movement: want (8,8) got (%d,%d)", col, row)
	}
}

func TestCHA(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[5;5H\x1b[40G") // CUP then CHA col=40
	col, row := cursorAt(t, term)
	if col != 39 || row != 4 {
		t.Fatalf("CHA: want (39,4) got (%d,%d)", col, row)
	}
}

// ── Erase ─────────────────────────────────────────────────────────────────────

func TestEraseInLine(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "HELLO")     // row 0: HELLO at 0-4
	feed(t, term, "\x1b[1;3H") // cursor to col 2 (0-based: 2)
	feed(t, term, "\x1b[K")    // EL 0: erase to end of line
	for col := 2; col < 10; col++ {
		if c := cellAt(t, term, 0, col); c.Char != ' ' {
			t.Fatalf("EL0: want space at col %d got %q", col, c.Char)
		}
	}
	if c := cellAt(t, term, 0, 0); c.Char != 'H' {
		t.Fatalf("EL0: H at col 0 should remain, got %q", c.Char)
	}
}

func TestEraseInDisplay_Below(t *testing.T) {
	term := newTerm(5, 3)
	feed(t, term, "AAAAABBBBBCCCCC") // 3 rows of 5 chars
	feed(t, term, "\x1b[2;3H")       // cursor row=1 col=2
	feed(t, term, "\x1b[J")          // ED 0: erase below + rest of line
	// row 1, cols 2-4 should be blank
	for col := 2; col < 5; col++ {
		if c := cellAt(t, term, 1, col); c.Char != ' ' {
			t.Fatalf("ED0: row1 col%d want space got %q", col, c.Char)
		}
	}
	// row 2 should be all blank
	for col := 0; col < 5; col++ {
		if c := cellAt(t, term, 2, col); c.Char != ' ' {
			t.Fatalf("ED0: row2 col%d want space got %q", col, c.Char)
		}
	}
	// row 0 should be untouched
	if c := cellAt(t, term, 0, 0); c.Char != 'A' {
		t.Fatalf("ED0: row0 col0 should be 'A' got %q", c.Char)
	}
}

// ── Scroll region ─────────────────────────────────────────────────────────────

func TestScrollRegion(t *testing.T) {
	// 6-row terminal so filling 5 rows with trailing \r\n does NOT trigger a
	// global scroll — row 0 stays AAAAA while we set up the test.
	term := newTerm(5, 6)
	for _, ch := range []string{"AAAAA", "BBBBB", "CCCCC", "DDDDD", "EEEEE"} {
		feed(t, term, ch+"\r\n")
	}
	// Scroll region: 1-based rows 2-4 = 0-based rows 1-3.
	// Rows 0 and 4 are outside and must not be affected.
	feed(t, term, "\x1b[2;4r")
	// Move cursor to bottom of region (1-based row 4 = 0-based row 3).
	feed(t, term, "\x1b[4;1H")
	// LF at scroll-region bottom: shifts rows 1-3 up by one, row 3 blanked.
	feed(t, term, "\n")
	// Row 0 outside region → must still be AAAAA.
	if c := cellAt(t, term, 0, 0); c.Char != 'A' {
		t.Fatalf("row0 outside region: want 'A' got %q", c.Char)
	}
	// Row 4 outside region → must still be EEEEE.
	if c := cellAt(t, term, 4, 0); c.Char != 'E' {
		t.Fatalf("row4 outside region: want 'E' got %q", c.Char)
	}
}

// ── Scrollback ────────────────────────────────────────────────────────────────

func TestScrollbackPush(t *testing.T) {
	term := newTerm(5, 2) // 2-row terminal, easy to trigger scroll
	feed(t, term, "AAAAA\r\nBBBBB\r\nCCCCC")
	// After 3 lines into a 2-row terminal, line 0 (AAAAA) should be in scrollback
	if term.ScrollbackLen() == 0 {
		t.Fatal("expected scrollback to be non-empty")
	}
	line := term.ScrollbackLine(term.ScrollbackLen() - 1)
	if line == nil || line[0].Char != 'A' {
		t.Fatalf("scrollback oldest line: want 'A', got %v", line)
	}
}

// ── Alt screen ────────────────────────────────────────────────────────────────

func TestAltScreen(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "PRIMARY")
	// Switch to alt screen
	feed(t, term, "\x1b[?1049h")
	// Alt screen should be blank
	if c := cellAt(t, term, 0, 0); c.Char != ' ' {
		t.Fatalf("alt screen col0 should be blank, got %q", c.Char)
	}
	feed(t, term, "ALTERNATE")
	// Switch back
	feed(t, term, "\x1b[?1049l")
	// Primary should have PRIMARY
	if c := cellAt(t, term, 0, 0); c.Char != 'P' {
		t.Fatalf("primary after return: want 'P' got %q", c.Char)
	}
}

// ── Resize ────────────────────────────────────────────────────────────────────

func TestResize(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "Hello")
	term.Resize(40, 12)
	term.RLock()
	cols, rows := term.Cols(), term.Rows()
	term.RUnlock()
	if cols != 40 || rows != 12 {
		t.Fatalf("after resize want (40,12) got (%d,%d)", cols, rows)
	}
	// Content that fit should survive
	if c := cellAt(t, term, 0, 0); c.Char != 'H' {
		t.Fatalf("after resize row0 col0 want 'H' got %q", c.Char)
	}
}

// ── OSC title ────────────────────────────────────────────────────────────────

func TestOSCTitle(t *testing.T) {
	term := newTerm(80, 24)
	var cbTitle string
	term.SetOnTitle(func(s string) { cbTitle = s })
	feed(t, term, "\x1b]0;My Terminal\x07") // onTitle called synchronously by feed
	term.RLock()
	title := term.Title()
	term.RUnlock()
	if title != "My Terminal" {
		t.Fatalf("title want %q got %q", "My Terminal", title)
	}
	if cbTitle != "My Terminal" {
		t.Fatalf("callback title want %q got %q", "My Terminal", cbTitle)
	}
}

// ── DCH / ICH ────────────────────────────────────────────────────────────────

func TestDCH(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "ABCDE")     // A B C D E _ _ _ _ _
	feed(t, term, "\x1b[1;2H") // cursor col 1 (0-based)
	feed(t, term, "\x1b[2P")   // delete 2 chars (B,C): A D E _ _ _ _ _ _ _
	if c := cellAt(t, term, 0, 1); c.Char != 'D' {
		t.Fatalf("DCH: want 'D' at col1 got %q", c.Char)
	}
}

func TestICH(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "ABCDE")     // A B C D E _ _ _ _ _
	feed(t, term, "\x1b[1;2H") // cursor col 1 (0-based)
	feed(t, term, "\x1b[2@")   // insert 2 blanks: A _ _ B C D E _ _ _
	if c := cellAt(t, term, 0, 1); c.Char != ' ' {
		t.Fatalf("ICH: want space at col1 got %q", c.Char)
	}
	if c := cellAt(t, term, 0, 3); c.Char != 'B' {
		t.Fatalf("ICH: want 'B' at col3 got %q", c.Char)
	}
}

// ── Dirty tracking ────────────────────────────────────────────────────────────

func TestDirtyTracking(t *testing.T) {
	term := newTerm(80, 24)
	// Print to row 0 only
	feed(t, term, "X")
	term.RLock()
	dirty0 := term.IsDirty(0)
	dirty1 := term.IsDirty(1)
	term.RUnlock()
	if !dirty0 {
		t.Fatal("row 0 should be dirty after print")
	}
	if dirty1 {
		t.Fatal("row 1 should not be dirty")
	}
	term.RLock()
	term.ClearDirty()
	dirty0after := term.IsDirty(0)
	term.RUnlock()
	if dirty0after {
		t.Fatal("row 0 should not be dirty after ClearDirty")
	}
}

// ── IL / DL ──────────────────────────────────────────────────────────────────

func TestIL(t *testing.T) {
	term := newTerm(5, 4)
	feed(t, term, "AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD")
	feed(t, term, "\x1b[2;1H") // cursor row 1
	feed(t, term, "\x1b[2L")   // insert 2 lines
	if c := cellAt(t, term, 1, 0); c.Char != ' ' {
		t.Fatalf("IL: row1 should be blank, got %q", c.Char)
	}
	if c := cellAt(t, term, 3, 0); c.Char != 'B' {
		t.Fatalf("IL: row3 should be 'B', got %q", c.Char)
	}
}

func TestDL(t *testing.T) {
	term := newTerm(5, 4)
	feed(t, term, "AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDDD")
	feed(t, term, "\x1b[2;1H") // cursor row 1
	feed(t, term, "\x1b[1M")   // delete 1 line
	if c := cellAt(t, term, 1, 0); c.Char != 'C' {
		t.Fatalf("DL: row1 should be 'C' after delete, got %q", c.Char)
	}
}

// ── Wide chars ────────────────────────────────────────────────────────────────

func TestWideChar(t *testing.T) {
	term := newTerm(10, 5)
	feed(t, term, "中") // U+4E2D, width=2
	col, row := cursorAt(t, term)
	if row != 0 || col != 2 {
		t.Fatalf("wide char: cursor want (2,0) got (%d,%d)", col, row)
	}
	if c := cellAt(t, term, 0, 0); c.Width != 2 {
		t.Fatalf("wide char: Cell.Width want 2 got %d", c.Width)
	}
}
