package gogpu

import (
	gogpulib "github.com/gogpu/gogpu"

	"github.com/termizard/termizard/internal/config"
)

// titleChromeHeightPhys is the macOS caption zone painted under a transparent
// title bar (FullSizeContentView). Zero when the OS draws chrome outside the
// content view (Windows / Linux / title bar disabled).
func (u *UI) titleChromeHeightPhys(ws *winState) int {
	if u == nil || u.cfg == nil || !u.cfg.Window.ShowTitleBar {
		return 0
	}
	if !isMac {
		return 0
	}
	sf := 1.0
	if ws != nil && ws.scaleFactor > 0 {
		sf = ws.scaleFactor
	}
	// Match Wails --chrome-titlebar-height (38 logical px).
	return int(38 * sf)
}

// headerAlignmentForConfig maps termizard title_position to gogpu HeaderAlignment.
// Center uses HeaderAlignRight so gogpu enables FullSizeContentView (colored
// title underlay) while we inject our own centered title in the painted chrome
// — native Center mode forces an opaque grey system title bar.
func headerAlignmentForConfig(cfg *config.Config) gogpulib.HeaderAlignment {
	if cfg == nil || !cfg.Window.ShowTitleBar {
		return gogpulib.HeaderAlignCenter
	}
	if !isMac {
		switch cfg.Window.TitlePosition {
		case config.TitleAlignRight:
			return gogpulib.HeaderAlignRight
		case config.TitleAlignLeft:
			return gogpulib.HeaderAlignLeft
		default:
			return gogpulib.HeaderAlignCenter
		}
	}
	// macOS: always FullSizeContent so we paint unified #1e1f22 under traffic lights.
	// Title text is drawn in paintTitleChrome (centered) to match config.
	return gogpulib.HeaderAlignRight
}

// blockLayout is the Fleet floating-block geometry.
type blockLayout struct {
	G         int // side gutters (physical px)
	BottomG   int // bottom gutter (equal to G)
	TabGap    int // thin chrome between tab bar and terminal
	TitleH    int
	TabH      int
	TabTop    int // -1 when tab bar hidden
	TermTop   int
	TermH     int
	TermW     int
	ChromeTop int // top of the chrome band above the terminal (applyEdgeChrome start)
}

// tabGapPhys is a small chrome hairline between tab bar and terminal (~4 logical px).
func (u *UI) tabGapPhys(ws *winState) int {
	sf := 1.0
	if ws != nil && ws.scaleFactor > 0 {
		sf = ws.scaleFactor
	}
	gap := int(4 * sf)
	if gap < 3 {
		gap = 3
	}
	g := u.termPadding(ws)
	if half := g / 2; half >= 2 && gap > half {
		gap = half
	}
	return gap
}

// blockLayout computes block positions. Title stays full-bleed (no gutters).
func (u *UI) blockLayout(ws *winState, fw, fh, numTabs int) blockLayout {
	g := u.termPadding(ws)
	bottomG := g // equal to side gutters (symmetric padding on all four sides)
	tabGap := 0
	if u.tabBarHeight(ws, numTabs) > 0 {
		tabGap = u.tabGapPhys(ws)
	}
	titleH := u.titleChromeHeightPhys(ws)
	tbH := u.tabBarHeight(ws, numTabs)

	var tabTop, termTop int
	switch {
	case titleH > 0 && tbH > 0:
		tabTop = titleH
		termTop = tabTop + tbH + tabGap
	case titleH > 0:
		tabTop = -1
		termTop = titleH + g
	case tbH > 0:
		tabTop = 0
		termTop = tbH + tabGap
	default:
		tabTop = -1
		termTop = g
	}

	// chromeTop: top of the chrome area above the terminal content.
	// With tabs: the gap between tab bar and terminal; connects to the tab bar chrome.
	// With title only: start below the painted title bar so the side chrome flows from there.
	// Neither: chrome starts at y=0 (full-bleed gutter).
	var chromeTop int
	switch {
	case tbH > 0:
		chromeTop = termTop - tabGap
	case titleH > 0:
		chromeTop = titleH
	default:
		chromeTop = 0
	}

	termH := fh - termTop - bottomG
	if termH < 0 {
		termH = 0
	}
	termW := fw - 2*g
	if termW < 0 {
		termW = 0
	}
	return blockLayout{
		G: g, BottomG: bottomG, TabGap: tabGap, TitleH: titleH, TabH: tbH, TabTop: tabTop,
		TermTop: termTop, TermH: termH, TermW: termW, ChromeTop: chromeTop,
	}
}

// titleAlignForPaint returns left/center/right for the software-drawn title.
func titleAlignForPaint(cfg *config.Config) string {
	if cfg == nil {
		return config.TitleAlignCenter
	}
	switch cfg.Window.TitlePosition {
	case config.TitleAlignLeft, config.TitleAlignRight, config.TitleAlignCenter:
		return cfg.Window.TitlePosition
	default:
		return config.TitleAlignCenter
	}
}
