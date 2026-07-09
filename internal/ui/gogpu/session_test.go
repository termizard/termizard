package gogpu

import (
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/adapter"
)

func TestTabSlotWriteMarksDirty(t *testing.T) {
	tab := newTabSlot(10, 3, 100, false)
	tab.write([]byte("hi"))
	if !tab.dirty.Load() {
		t.Fatal("dirty not set")
	}
	if tab.scrollOffset.Load() != 0 {
		t.Fatal("scroll offset should reset on write")
	}
}

func TestTabSlotSendInput(t *testing.T) {
	tab := newTabSlot(10, 3, 100, false)
	var got []byte
	tab.keyFn = func(e adapter.KeyEvent) { got = e.Data }
	tab.sendInput([]byte("x"))
	if string(got) != "x" {
		t.Fatalf("sendInput = %q", got)
	}
}

func TestExtractSelectedTextMultiRow(t *testing.T) {
	ws, tab := newWinState(10, 4, 100, false)
	tab.write([]byte("hello\r\nworld\r\n"))
	ws.mu.Lock()
	tab.selActive = true
	tab.selStartCol, tab.selStartRow = 1, 0
	tab.selEndCol, tab.selEndRow = 4, 1
	ws.mu.Unlock()
	got := ws.extractSelectedText()
	if got == "" {
		t.Fatal("empty selection text")
	}
	if got[0] != 'e' {
		t.Fatalf("selection starts with %q", got)
	}
}

func TestExtractSelectedTextTrimsTrailingSpaces(t *testing.T) {
	ws, tab := newWinState(10, 2, 100, false)
	tab.write([]byte("ab    \r\n"))
	ws.mu.Lock()
	tab.selActive = true
	tab.selStartCol, tab.selStartRow = 0, 0
	tab.selEndCol, tab.selEndRow = 9, 0
	ws.mu.Unlock()
	got := ws.extractSelectedText()
	if got != "ab" {
		t.Fatalf("trimmed = %q", got)
	}
}

func TestDestroyTexNilSafe(t *testing.T) {
	ws := &winState{}
	ws.destroyTex()
}

func TestVisualTabSlotDrag(t *testing.T) {
	if got := visualTabSlot(2, 0, 2); got != 1 {
		t.Fatalf("slot = %d, want 1", got)
	}
	if got := visualTabSlot(0, 0, 2); got != 0 {
		t.Fatalf("dragged slot = %d", got)
	}
}

func TestReorderTabSlotsMoves(t *testing.T) {
	tabs := []*tabSlot{newTabSlot(1, 1, 1, false), newTabSlot(1, 1, 1, false), newTabSlot(1, 1, 1, false)}
	out := reorderTabSlots(tabs, 0, 2)
	if len(out) != 3 {
		t.Fatalf("len = %d", len(out))
	}
}

func TestTabIndexAtXInStrip(t *testing.T) {
	fw, inset, cellW := 800, 24, 10
	idx := tabIndexAtX(inset+50, fw, inset, 3, cellW, 2.0, "compact", 0, 0)
	if idx < 0 {
		t.Fatalf("idx = %d", idx)
	}
}

func TestTabIndexAtXOutsideStrip(t *testing.T) {
	fw, inset := 800, 24
	idx := tabIndexAtX(5, fw, inset, 3, 10, 2.0, "compact", 0, 0)
	if idx != -1 {
		t.Fatalf("outside strip idx = %d", idx)
	}
}

func TestShortenTabTitlePath(t *testing.T) {
	got := shortenTabTitle("/Users/me/projects/termizard")
	if got == "" || got == "/Users/me/projects/termizard" {
		t.Fatalf("shorten = %q", got)
	}
}

func TestShortenTabTitleLong(t *testing.T) {
	long := strings.Repeat("a", 60)
	got := shortenTabTitle(long)
	if len([]rune(got)) > 41 {
		t.Fatalf("too long: %q", got)
	}
}

func TestTabIndexAtXPinnedRight(t *testing.T) {
	const fw = 400
	const inset = 0
	const cellW = 8
	const numTabs = 6
	tabW, _, tabsArea := computeTabLayout(fw, inset, numTabs, cellW, 1, "compact")
	scroll := tabScrollMax(numTabs, tabW, tabsArea)
	active := numTabs - 1
	_, pinR := activeTabPin(active, numTabs, tabW, tabsArea, scroll)
	if !pinR {
		t.Skip("need overflow for pin-right")
	}
	pinX := tabsArea - tabW
	if got := tabIndexAtX(pinX+2, fw, inset, numTabs, cellW, 1, "compact", scroll, active); got != active {
		t.Fatalf("pinned right = %d, want %d", got, active)
	}
}
