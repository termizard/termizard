package gogpu

import (
	"runtime"
	"testing"

	gogpulib "github.com/gogpu/gogpu"

	"github.com/termizard/termizard/internal/config"
)

// titleAlignForPaint

func TestTitleAlignForPaintNilConfig(t *testing.T) {
	got := titleAlignForPaint(nil)
	if got != config.TitleAlignCenter {
		t.Fatalf("nil cfg: got %q, want center", got)
	}
}

func TestTitleAlignForPaintCenter(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = config.TitleAlignCenter
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignCenter {
		t.Fatalf("got %q, want center", got)
	}
}

func TestTitleAlignForPaintLeft(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = config.TitleAlignLeft
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignLeft {
		t.Fatalf("got %q, want left", got)
	}
}

func TestTitleAlignForPaintRight(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = config.TitleAlignRight
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignRight {
		t.Fatalf("got %q, want right", got)
	}
}

func TestTitleAlignForPaintUnknownDefaultsToCenter(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = "diagonal"
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignCenter {
		t.Fatalf("unknown position: got %q, want center", got)
	}
}

func TestHeaderAlignmentNilConfig(t *testing.T) {
	if got := headerAlignmentForConfig(nil); got != gogpulib.HeaderAlignCenter {
		t.Fatalf("nil cfg alignment = %v", got)
	}
}

func TestHeaderAlignmentTitleBarDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.ShowTitleBar = false
	if got := headerAlignmentForConfig(cfg); got != gogpulib.HeaderAlignCenter {
		t.Fatalf("disabled title bar = %v", got)
	}
}

func TestHeaderAlignmentNonMacPositions(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("non-mac positions only on other OS")
	}
	cfg := &config.Config{Window: config.WindowConfig{ShowTitleBar: true, TitlePosition: config.TitleAlignLeft}}
	if got := headerAlignmentForConfig(cfg); got != gogpulib.HeaderAlignLeft {
		t.Fatalf("left = %v", got)
	}
	cfg.Window.TitlePosition = config.TitleAlignRight
	if got := headerAlignmentForConfig(cfg); got != gogpulib.HeaderAlignRight {
		t.Fatalf("right = %v", got)
	}
}

func TestHeaderAlignmentMacShowTitleBar(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS header alignment")
	}
	cfg := &config.Config{}
	cfg.Window.ShowTitleBar = true
	cfg.Window.TitlePosition = config.TitleAlignLeft
	if got := headerAlignmentForConfig(cfg); got != gogpulib.HeaderAlignRight {
		t.Fatalf("mac FullSizeContent = %v", got)
	}
}

func TestTitleChromeHeightPhys(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Window.ShowTitleBar = true
	if runtime.GOOS == "darwin" {
		if got := u.titleChromeHeightPhys(ws); got <= 0 {
			t.Fatalf("darwin titleH = %d", got)
		}
	} else {
		if got := u.titleChromeHeightPhys(ws); got != 0 {
			t.Fatalf("non-darwin titleH = %d, want 0", got)
		}
	}
	u.cfg.Window.ShowTitleBar = false
	if got := u.titleChromeHeightPhys(ws); got != 0 {
		t.Fatalf("disabled titleH = %d", got)
	}
}

func TestTabGapPhys(t *testing.T) {
	u, ws := testUI(t)
	gap := u.tabGapPhys(ws)
	if gap < 3 {
		t.Fatalf("tabGap = %d, too small", gap)
	}
	g := u.termPadding(ws)
	if gap > g/2 && g/2 >= 2 {
		t.Fatalf("tabGap %d exceeds half gutter %d", gap, g/2)
	}
}

func TestBlockLayoutTitleAndTabs(t *testing.T) {
	u, ws := testUI(t)
	ws.scaleFactor = 2.0
	ws.cellH = 20
	bl := u.blockLayout(ws, 1200, 800, 2)
	if bl.TabTop <= 0 && u.titleChromeHeightPhys(ws) > 0 {
		t.Fatalf("TabTop = %d with title", bl.TabTop)
	}
	if bl.TermTop != bl.TabTop+bl.TabH+bl.TabGap {
		t.Fatalf("TermTop = %d, want TabTop+TabH+TabGap (%d+%d+%d)",
			bl.TermTop, bl.TabTop, bl.TabH, bl.TabGap)
	}
	if bl.TermW != 1200-2*bl.G {
		t.Fatalf("TermW = %d, want %d", bl.TermW, 1200-2*bl.G)
	}
	if bl.TermH != 800-bl.TermTop-bl.BottomG {
		t.Fatalf("TermH = %d", bl.TermH)
	}
	if bl.ChromeTop != bl.TermTop-bl.TabGap {
		t.Fatalf("ChromeTop = %d, want %d", bl.ChromeTop, bl.TermTop-bl.TabGap)
	}
}

func TestBlockLayoutNoTabs(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Tabs.Enabled = false
	bl := u.blockLayout(ws, 800, 600, 1)
	if bl.TabH != 0 {
		t.Fatalf("TabH = %d with tabs disabled", bl.TabH)
	}
	if bl.TabGap != 0 {
		t.Fatalf("TabGap = %d without tab bar", bl.TabGap)
	}
}
