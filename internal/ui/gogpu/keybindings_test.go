package gogpu

import (
	"image/color"
	"testing"

	"github.com/gogpu/gpucontext"
)

func TestMatchPlatformBindingMacPaste(t *testing.T) {
	action := matchPlatformBinding(gpucontext.KeyV, gpucontext.ModSuper, true)
	if action != actionPaste {
		t.Fatalf("matchPlatformBinding(Cmd+V) = %q, want Paste", action)
	}
}

func TestMatchPlatformBindingWindowsPaste(t *testing.T) {
	action := matchPlatformBinding(gpucontext.KeyV, gpucontext.ModControl, false)
	if action != actionPaste {
		t.Fatalf("matchPlatformBinding(Ctrl+V) = %q, want Paste", action)
	}
}

func TestMatchPlatformBindingIgnoresCapsLock(t *testing.T) {
	action := matchPlatformBinding(gpucontext.KeyC, gpucontext.ModSuper|gpucontext.ModCapsLock, true)
	if action != actionCopy {
		t.Fatalf("Cmd+C with CapsLock = %q, want Copy", action)
	}
}

func TestClipboardEqual(t *testing.T) {
	if !clipboardEqual("a\r\nb", "a\nb") {
		t.Fatal("expected newline-normalized equality")
	}
}

func TestReorderTabSlots(t *testing.T) {
	tabs := []*tabSlot{{}, {}, {}, {}}
	got := reorderTabSlots(tabs, 0, 2)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[2] != tabs[0] {
		t.Fatal("expected first tab at index 2")
	}

	got = reorderTabSlots(tabs, 3, 0)
	if got[0] != tabs[3] {
		t.Fatal("expected last tab at index 0")
	}
}

func TestVisualTabSlot(t *testing.T) {
	// Drag tab 0 toward slot 2: tabs 1,2 shift left.
	if got := visualTabSlot(1, 0, 2); got != 0 {
		t.Fatalf("visualTabSlot(1,0,2) = %d, want 0", got)
	}
	if got := visualTabSlot(2, 0, 2); got != 1 {
		t.Fatalf("visualTabSlot(2,0,2) = %d, want 1", got)
	}
	// Drag tab 3 toward slot 0: tabs 0,1,2 shift right.
	if got := visualTabSlot(0, 3, 0); got != 1 {
		t.Fatalf("visualTabSlot(0,3,0) = %d, want 1", got)
	}
}

func TestTabIndexAtX(t *testing.T) {
	const fw = 400
	const cellW = 8
	const numTabs = 4
	const inset = 0
	tabW, _, _ := computeTabLayout(fw, inset, numTabs, cellW, 1, "full")

	if got := tabIndexAtX(tabW/4, fw, inset, numTabs, cellW, 1, "full", 0, 0); got != 0 {
		t.Fatalf("left quarter = %d, want 0", got)
	}
	if got := tabIndexAtX(tabW+tabW/4, fw, inset, numTabs, cellW, 1, "full", 0, 0); got != 1 {
		t.Fatalf("second tab = %d, want 1", got)
	}
}

func TestActiveTabPinHit(t *testing.T) {
	const fw = 400
	const cellW = 8
	const numTabs = 8
	const inset = 0
	tabW, _, tabsArea := computeTabLayout(fw, inset, numTabs, cellW, 1, "compact")
	if tabsArea >= numTabs*tabW {
		t.Fatalf("expected overflow: tabsArea=%d total=%d", tabsArea, numTabs*tabW)
	}
	// Scroll so active tab 0 is cut on the left → pin-left; left edge hits active.
	scroll := tabW * 2
	pinL, pinR := activeTabPin(0, numTabs, tabW, tabsArea, scroll)
	if !pinL || pinR {
		t.Fatalf("pin = (%v,%v), want left", pinL, pinR)
	}
	if got := tabIndexAtX(4, fw, inset, numTabs, cellW, 1, "compact", scroll, 0); got != 0 {
		t.Fatalf("pinned left hit = %d, want 0", got)
	}
}

func TestComputeTabLayoutStretch(t *testing.T) {
	tabW, _, _ := computeTabLayout(800, 0, 2, 8, 1, "full")
	if tabW < 300 {
		t.Fatalf("tabW = %d, want stretch (~391)", tabW)
	}
}

func TestComputeTabLayoutCompact(t *testing.T) {
	tabW, plusX, tabsArea := computeTabLayout(1200, 12, 2, 8, 1, "compact")
	if tabW != 200 {
		t.Fatalf("compact tabW = %d, want 200", tabW)
	}
	if tabsArea >= 1200-24 {
		t.Fatalf("tabsArea should leave room for + button, got %d", tabsArea)
	}
	if plusX < 12 {
		t.Fatalf("plusX = %d, want inset-aligned (>=12)", plusX)
	}
}

func TestComputeTabLayoutInsetAlign(t *testing.T) {
	const inset = 20
	const fw = 1000
	const cellW = 10
	_, plusX, tabsArea := computeTabLayout(fw, inset, 3, cellW, 1, "compact")
	plusW := plusBtnWidth(cellW, 1)
	contentR := fw - inset
	plusZoneL := contentR - plusW
	if inset+tabsArea+plusW != contentR {
		t.Fatalf("row width = %d, want content right %d", inset+tabsArea+plusW, contentR)
	}
	if plusX < plusZoneL || plusX+cellW > contentR {
		t.Fatalf("+ out of zone: plusX=%d zone=[%d,%d]", plusX, plusZoneL, contentR)
	}
}

func TestShortenTabTitle(t *testing.T) {
	got := shortenTabTitle("/Users/vlad/Workspace/opensource/gogpu-projects/wgpu-local")
	if got != "…/gogpu-projects/wgpu-local" {
		t.Fatalf("shorten = %q", got)
	}
	if got := shortenTabTitle("caffeinate"); got != "caffeinate" {
		t.Fatalf("process title = %q", got)
	}
}

func TestFillRectNegativeClip(t *testing.T) {
	// Dragging a tab left produces negative x — must not panic.
	buf := make([]byte, 100*20*4)
	fillRect(buf, 100, -50, 0, 20, 20, color.RGBA{R: 1, G: 2, B: 3, A: 255})
}
