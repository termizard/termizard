package terminal_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/core/vte"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newTerm(cols, rows int) *terminal.Terminal {
	return terminal.New(cols, rows, 1000, true)
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
	return term.Cell(row, col)
}

func cursorAt(t *testing.T, term *terminal.Terminal) (col, row int) {
	t.Helper()
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
	if c.FG.Value != 255<<16|128<<8 {
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
	if len(line) == 0 || line[0].Char != 'A' {
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
	cols, rows := term.Cols(), term.Rows()
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
	title := term.Title()
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
	dirty0 := term.IsDirty(0)
	dirty1 := term.IsDirty(1)
	if !dirty0 {
		t.Fatal("row 0 should be dirty after print")
	}
	if dirty1 {
		t.Fatal("row 1 should not be dirty")
	}
	term.ClearDirty()
	dirty0after := term.IsDirty(0)
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

// ── Resize: column overflow preservation ─────────────────────────────────────

// TestResizePreservesOverflowCols verifies that shrinking the column count does
// not permanently discard cells beyond the new display width. Content written
// when the grid was wider must survive a shrink-then-expand cycle — this is the
// "matrix" storage model that prevents truncation during transient wrong-size
// resize events (e.g. wrong dc.Width() on first draw frame).
func TestResizePreservesOverflowCols(t *testing.T) {
	// 70-col terminal; position cursor at col 50 and write a sentinel char.
	term := newTerm(70, 5)
	feed(t, term, "\x1b[1;51H") // CUP row=1 col=51 (1-based) → row=0, col=50
	feed(t, term, "Z")

	if c := cellAt(t, term, 0, 50); c.Char != 'Z' {
		t.Fatalf("before resize: want 'Z' at (0,50) got %q", c.Char)
	}

	// Shrink to 35 cols — 'Z' at col 50 is now beyond the display width.
	term.Resize(35, 5)
	if c := term.Cols(); c != 35 {
		t.Fatalf("after shrink: want 35 cols got %d", c)
	}

	// Expand back to 70 cols — 'Z' must still be at col 50.
	term.Resize(70, 5)
	if c := cellAt(t, term, 0, 50); c.Char != 'Z' {
		t.Fatalf("after expand: want 'Z' at (0,50) preserved, got %q", c.Char)
	}
}

// ── Resize: soft-wrap reflow ──────────────────────────────────────────────────

// TestSoftWrapMergeOnExpand verifies that expanding the column count merges
// soft-wrapped continuation rows back into a single logical line. A 75-char
// string written to a 70-col terminal occupies two visual rows (70 + 5). After
// widening to 80 cols, all 75 chars must appear on row 0 and row 1 must be blank.
func TestSoftWrapMergeOnExpand(t *testing.T) {
	term := newTerm(70, 10)
	feed(t, term, strings.Repeat("A", 75))

	if c := cellAt(t, term, 0, 69); c.Char != 'A' {
		t.Fatalf("before resize: want 'A' at (0,69) got %q", c.Char)
	}
	if c := cellAt(t, term, 1, 4); c.Char != 'A' {
		t.Fatalf("before resize: want 'A' at (1,4) got %q", c.Char)
	}

	term.Resize(80, 10)

	for col := 0; col < 75; col++ {
		if c := cellAt(t, term, 0, col); c.Char != 'A' {
			t.Fatalf("after expand: want 'A' at (0,%d) got %q", col, c.Char)
		}
	}
	if c := cellAt(t, term, 1, 0); c.Char != ' ' {
		t.Fatalf("after expand: row 1 col 0 should be blank, got %q", c.Char)
	}
}

// TestSoftWrapRewrapOnShrink verifies that shrinking breaks a long line into
// multiple wrapped rows so no content is lost.
func TestSoftWrapRewrapOnShrink(t *testing.T) {
	term := newTerm(80, 10)
	feed(t, term, strings.Repeat("B", 75))

	term.Resize(40, 10)

	// First 40 B's on row 0, next 35 B's on row 1.
	for col := 0; col < 40; col++ {
		if c := cellAt(t, term, 0, col); c.Char != 'B' {
			t.Fatalf("after shrink: want 'B' at (0,%d) got %q", col, c.Char)
		}
	}
	for col := 0; col < 35; col++ {
		if c := cellAt(t, term, 1, col); c.Char != 'B' {
			t.Fatalf("after shrink: want 'B' at (1,%d) got %q", col, c.Char)
		}
	}
	if c := cellAt(t, term, 1, 35); c.Char != ' ' {
		t.Fatalf("after shrink: col 35 row 1 should be blank, got %q", c.Char)
	}
}

// TestResizeRejoinsSplitWord verifies that a logical line split across saved
// prefix and visible grid tail (e.g. "Mov" + "ies") is rejoined on expand.
func TestResizeRejoinsSplitWord(t *testing.T) {
	term := newTerm(80, 5)
	line := strings.Repeat("x", 65) + "Movies"
	feed(t, term, line)

	term.Resize(20, 5)
	term.Resize(80, 5)

	var text strings.Builder
	for row := 0; row < 5; row++ {
		for col := 0; col < 80; col++ {
			c := cellAt(t, term, row, col)
			if c.Char != 0 && c.Char != ' ' {
				text.WriteRune(c.Char)
			}
		}
	}
	got := text.String()
	if !strings.Contains(got, "Movies") {
		t.Fatalf("want intact Movies in %q", got)
	}
	if strings.Contains(got, "Mov") && strings.Contains(got, "ies") && !strings.Contains(got, "Movies") {
		t.Fatalf("Movies split across rows: %q", got)
	}
}

// TestResizeLsStyleOutput simulates directory listing lines that must stay
// separate across narrow→wide resize cycles.
func TestResizeLsStyleOutput(t *testing.T) {
	term := newTerm(120, 24)
	lines := "total 88\r\n" +
		"drwx------@  Desktop\r\n" +
		"drwx------+  Dev\r\n" +
		"drwx------@  Documents\r\n" +
		"drwx------@  Downloads\r\n" +
		"drwx------   Library\r\n"
	feed(t, term, lines)

	term.Resize(40, 24)
	term.Resize(120, 24)

	for row, want := range []string{"total", "drwx", "drwx", "drwx", "drwx", "drwx"} {
		c := cellAt(t, term, row, 0)
		if c.Char != rune(want[0]) {
			t.Fatalf("row %d: want line starting with %q, got %q", row, want, string(c.Char))
		}
	}
}

// TestResizePreservesMultipleLinesOnExpand verifies that hard line breaks are
// not merged when reflow overflow is saved and restored across resize cycles.
func TestResizePreservesMultipleLinesOnExpand(t *testing.T) {
	term := newTerm(80, 5)
	feed(t, term, "AAAAA\r\nBBBBB\r\n"+strings.Repeat("C", 30)+"\r\nDDDDD\r\n")

	term.Resize(10, 5) // line C wraps; 6 visual rows → skip 1
	term.Resize(80, 5)

	if c := cellAt(t, term, 0, 0); c.Char != 'A' {
		t.Fatalf("row0 want A got %q", c.Char)
	}
	if c := cellAt(t, term, 1, 0); c.Char != 'B' {
		t.Fatalf("row1 want B got %q", c.Char)
	}
	for row := 0; row < 5; row++ {
		if cellAt(t, term, row, 0).Char == 'D' {
			if cellAt(t, term, row, 4).Char != 'D' {
				t.Fatalf("row%d: DDDDD corrupted: %q", row, cellAt(t, term, row, 0).Char)
			}
			return
		}
	}
	t.Fatal("DDDDD row not found")
}

// TestSoftWrapExpandRestoresAfterOverflowShrink verifies that reflow overflow
// during a width shrink does not permanently discard the top of long lines.
// When the window is widened again, the full line must reappear on screen.
func TestSoftWrapExpandRestoresAfterOverflowShrink(t *testing.T) {
	term := newTerm(80, 5)
	feed(t, term, strings.Repeat("X", 200))

	term.Resize(30, 5) // 7 visual rows needed at 30 cols; 2 scroll off
	term.Resize(80, 5) // merge back to 3 rows; all 200 chars must be visible

	count := 0
	for row := 0; row < 5; row++ {
		for col := 0; col < 80; col++ {
			if cellAt(t, term, row, col).Char == 'X' {
				count++
			}
		}
	}
	if count != 200 {
		t.Fatalf("after shrink→expand: want 200 X cells visible, got %d", count)
	}
	if c := cellAt(t, term, 0, 0); c.Char != 'X' {
		t.Fatalf("after shrink→expand: row0 col0 want 'X' got %q", c.Char)
	}
}

// TestVerticalShrinkExpandRoundTrip verifies that shrinking the row count
// preserves content in reflowSavedLines and a subsequent expand restores it.
func TestVerticalShrinkExpandRoundTrip(t *testing.T) {
	term := newTerm(80, 24)
	for i := 0; i < 20; i++ {
		feed(t, term, fmt.Sprintf("line%02d\r\n", i))
	}

	term.Resize(80, 10)
	if c := cellAt(t, term, 0, 0); c.Char != 'l' {
		t.Fatalf("after shrink: bottom lines should remain visible, row0 got %q", c.Char)
	}

	term.Resize(80, 24)
	for row, want := range map[int]string{0: "line00", 10: "line10", 19: "line19"} {
		got := string(cellAt(t, term, row, 0).Char)
		for col := 1; col < len(want); col++ {
			got += string(cellAt(t, term, row, col).Char)
		}
		if got != want {
			t.Fatalf("after expand row %d: want %q got %q", row, want, got)
		}
	}
}

// TestVerticalResizeDoesNotGrowScrollback verifies reflow resize does not push
// lines into scrollback on every intermediate height change during a drag.
func TestVerticalResizeDoesNotGrowScrollback(t *testing.T) {
	term := newTerm(80, 24)
	for i := 0; i < 20; i++ {
		feed(t, term, fmt.Sprintf("line%02d\r\n", i))
	}
	before := term.ScrollbackLen()
	for h := 24; h >= 10; h-- {
		term.Resize(80, h)
	}
	for h := 10; h <= 24; h++ {
		term.Resize(80, h)
	}
	if got := term.ScrollbackLen(); got != before {
		t.Fatalf("scrollback grew from %d to %d during vertical resize reflow", before, got)
	}
}

// TestVerticalShrinkExpandCursorAtTop verifies content is preserved even when
// the cursor sits on the first line while many lines are below it.
func TestVerticalShrinkExpandCursorAtTop(t *testing.T) {
	term := newTerm(80, 24)
	feed(t, term, "\x1b[1;1H") // cursor row 0
	for i := 0; i < 20; i++ {
		feed(t, term, fmt.Sprintf("line%02d\r\n", i))
	}
	feed(t, term, "\x1b[1;1H")

	term.Resize(80, 10)
	term.Resize(80, 24)

	if c := cellAt(t, term, 0, 0); c.Char != 'l' {
		t.Fatalf("after round-trip row0 want line00, got %q", c.Char)
	}
	if c := cellAt(t, term, 19, 0); c.Char != 'l' {
		t.Fatalf("after round-trip row19 want line19, got %q", c.Char)
	}
}

// original row layout from reflowed content.
func TestSoftWrapRoundTrip(t *testing.T) {
	term := newTerm(80, 10)
	feed(t, term, strings.Repeat("C", 75))

	term.Resize(40, 10)
	term.Resize(80, 10)

	// All 75 C's back on row 0.
	for col := 0; col < 75; col++ {
		if c := cellAt(t, term, 0, col); c.Char != 'C' {
			t.Fatalf("after round-trip: want 'C' at (0,%d) got %q", col, c.Char)
		}
	}
	if c := cellAt(t, term, 1, 0); c.Char != ' ' {
		t.Fatalf("after round-trip: row 1 should be blank, got %q", c.Char)
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
