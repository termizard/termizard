package gogpu

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/termizard/termizard/internal/config"
)

// selectionRange.contains

func TestSelectionRangeContainsMiddleRow(t *testing.T) {
	sr := &selectionRange{c0: 2, r0: 1, c1: 5, r1: 3}
	if !sr.contains(0, 2) {
		t.Fatal("middle row should be fully inside")
	}
}

func TestSelectionRangeContainsStartRow(t *testing.T) {
	sr := &selectionRange{c0: 3, r0: 1, c1: 5, r1: 3}
	if sr.contains(2, 1) {
		t.Fatal("col 2 < c0=3 on start row should be outside")
	}
	if !sr.contains(3, 1) {
		t.Fatal("col 3 = c0=3 should be inside")
	}
}

func TestSelectionRangeContainsEndRow(t *testing.T) {
	sr := &selectionRange{c0: 0, r0: 0, c1: 5, r1: 2}
	if sr.contains(6, 2) {
		t.Fatal("col 6 > c1=5 on end row should be outside")
	}
	if !sr.contains(5, 2) {
		t.Fatal("col 5 = c1=5 should be inside")
	}
}

func TestSelectionRangeContainsBeforeStart(t *testing.T) {
	sr := &selectionRange{c0: 0, r0: 2, c1: 0, r1: 4}
	if sr.contains(0, 1) {
		t.Fatal("row 1 < r0=2 should be outside")
	}
}

func TestSelectionRangeContainsAfterEnd(t *testing.T) {
	sr := &selectionRange{c0: 0, r0: 0, c1: 0, r1: 2}
	if sr.contains(0, 3) {
		t.Fatal("row 3 > r1=2 should be outside")
	}
}

func TestSelectionRangeNilSafe(t *testing.T) {
	var sr *selectionRange
	if sr.contains(0, 0) {
		t.Fatal("nil selectionRange should always return false")
	}
}

// blendRGBA

func TestBlendRGBAt0(t *testing.T) {
	a := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	b := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	got := blendRGBA(a, b, 0)
	if got.R != 255 || got.G != 0 {
		t.Fatalf("t=0 should return a: %v", got)
	}
}

func TestBlendRGBAt1(t *testing.T) {
	a := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	b := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	got := blendRGBA(a, b, 1)
	if got.R != 0 || got.G != 255 {
		t.Fatalf("t=1 should return b: %v", got)
	}
}

func TestBlendRGBAt05(t *testing.T) {
	a := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	b := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	got := blendRGBA(a, b, 0.5)
	if got.R < 98 || got.R > 102 {
		t.Fatalf("midpoint R out of range: %d", got.R)
	}
}

func TestBlendRGBAClampsBelowZero(t *testing.T) {
	a := color.RGBA{R: 10, A: 255}
	b := color.RGBA{R: 20, A: 255}
	got := blendRGBA(a, b, -1)
	if got.R != 10 {
		t.Fatalf("t<0 should clamp to 0: %v", got)
	}
}

func TestBlendRGBAClampsAbove1(t *testing.T) {
	a := color.RGBA{R: 10, A: 255}
	b := color.RGBA{R: 20, A: 255}
	got := blendRGBA(a, b, 2)
	if got.R != 20 {
		t.Fatalf("t>1 should clamp to 1: %v", got)
	}
}

// dimColor

func TestDimColorSubtracts(t *testing.T) {
	c := color.RGBA{R: 100, G: 50, B: 200, A: 255}
	got := dimColor(c, 50)
	if got.R != 50 || got.G != 0 || got.B != 150 || got.A != 255 {
		t.Fatalf("got %v", got)
	}
}

func TestDimColorClampsToZero(t *testing.T) {
	c := color.RGBA{R: 10, G: 0, B: 5, A: 255}
	got := dimColor(c, 255)
	if got.R != 0 || got.G != 0 || got.B != 0 {
		t.Fatalf("should clamp to zero: %v", got)
	}
}

func TestDimColorPreservesAlpha(t *testing.T) {
	c := color.RGBA{R: 100, G: 100, B: 100, A: 128}
	got := dimColor(c, 10)
	if got.A != 128 {
		t.Fatalf("alpha changed: %d", got.A)
	}
}

// min3

func TestMin3AllEqual(t *testing.T) {
	if min3(5, 5, 5) != 5 {
		t.Fatal("all equal")
	}
}

func TestMin3FirstSmallest(t *testing.T) {
	if min3(1, 2, 3) != 1 {
		t.Fatal("first smallest")
	}
}

func TestMin3SecondSmallest(t *testing.T) {
	if min3(5, 2, 8) != 2 {
		t.Fatal("second smallest")
	}
}

func TestMin3ThirdSmallest(t *testing.T) {
	if min3(5, 8, 3) != 3 {
		t.Fatal("third smallest")
	}
}

// chromeAtXY

func TestChromeAtXYTopLeft(t *testing.T) {
	c := chromeAtXY(0, 0, 100, 100)
	// t=0 → chromeGreen end of the gradient.
	if c.R == chromeGray.R && c.G == chromeGray.G && c.B == chromeGray.B {
		t.Fatalf("top-left should be green end, got gray: %v", c)
	}
}

func TestChromeAtXYBottomRight(t *testing.T) {
	c := chromeAtXY(99, 99, 100, 100)
	// t≈1 → blends toward chromeGray
	_ = c // must not panic
}

func TestChromeAtXYClampNegative(t *testing.T) {
	// negative coords should not panic
	c := chromeAtXY(-10, -10, 100, 100)
	_ = c
}

func TestChromeAtXYClampBeyondFrame(t *testing.T) {
	c := chromeAtXY(200, 200, 100, 100)
	_ = c
}

func TestChromeAtXYTinyFrame(t *testing.T) {
	c := chromeAtXY(0, 0, 1, 1)
	if c != chromeGreen {
		t.Fatalf("1×1 frame should return chromeGreen: %v", c)
	}
}

// fillRect (more cases)

func TestFillRectFillsCorrectly(t *testing.T) {
	const fw = 10
	buf := make([]byte, fw*10*4)
	red := color.RGBA{R: 255, A: 255}
	fillRect(buf, fw, 2, 3, 5, 6, red)
	// pixel (2, 3) should be red
	off := (3*fw + 2) * 4
	if buf[off] != 255 || buf[off+3] != 255 {
		t.Fatalf("pixel (2,3) not red: %v", buf[off:off+4])
	}
	// pixel (0, 0) should be untouched
	if buf[0] != 0 {
		t.Fatal("pixel (0,0) should not be touched")
	}
}

func TestFillRectEmptyRange(t *testing.T) {
	buf := make([]byte, 100*4)
	fillRect(buf, 100, 5, 0, 5, 1, color.RGBA{R: 1, A: 255}) // x0 == x1
	if buf[5*4] != 0 {
		t.Fatal("empty x range should not write")
	}
}

func TestFillRectZeroFrameWidth(t *testing.T) {
	buf := make([]byte, 100)
	fillRect(buf, 0, 0, 0, 10, 10, color.RGBA{R: 1, A: 255}) // must not panic
}

// fillRoundRect

func TestFillRoundRectNoPanic(t *testing.T) {
	buf := make([]byte, 100*100*4)
	fillRoundRect(buf, 100, 10, 10, 50, 50, 8, color.RGBA{R: 200, G: 100, B: 50, A: 255})
}

func TestFillRoundRectZeroRadius(t *testing.T) {
	buf := make([]byte, 100*100*4)
	fillRoundRect(buf, 100, 0, 0, 10, 10, 0, color.RGBA{R: 255, A: 255})
	// center pixel should be filled
	off := (5*100 + 5) * 4
	if buf[off] != 255 {
		t.Fatalf("center not filled")
	}
}

func TestFillRoundRectEmptyRange(t *testing.T) {
	buf := make([]byte, 100*100*4)
	fillRoundRect(buf, 100, 10, 10, 5, 5, 4, color.RGBA{R: 1, A: 255}) // x0>x1: no-op
}

func TestFillRoundRectLargeRadius(t *testing.T) {
	// radius > half-width/height: should not panic
	buf := make([]byte, 200*200*4)
	fillRoundRect(buf, 200, 10, 10, 20, 20, 100, color.RGBA{R: 1, A: 255})
}

// drawBeam

func TestDrawBeam(t *testing.T) {
	const fw = 20
	buf := make([]byte, fw*20*4)
	fg := color.RGBA{R: 255, G: 128, B: 0, A: 255}
	drawBeam(buf, fw, 5, 2, 8, fg)
	// pixel (5, 2) should be set
	off := (2*fw + 5) * 4
	if buf[off] != 255 || buf[off+1] != 128 {
		t.Fatalf("beam pixel not written: %v", buf[off:off+4])
	}
}

func TestDrawBeamOutOfBounds(t *testing.T) {
	// y1 beyond buffer — must stop, not panic
	buf := make([]byte, 10*4) // only 1 row of 10 px
	drawBeam(buf, 10, 0, 0, 100, color.RGBA{R: 1, A: 255})
}

// drawUnderline

func TestDrawUnderline(t *testing.T) {
	const fw = 20
	const cH = 16
	buf := make([]byte, fw*cH*4)
	fg := color.RGBA{G: 255, A: 255}
	drawUnderline(buf, fw, 0, 5, 0, cH, fg)
	// underline is at the bottom 2 pixels of the cell
	barY0 := cH - 2
	off := (barY0*fw + 0) * 4
	if buf[off+1] != 255 {
		t.Fatalf("underline not drawn: %v", buf[off:off+4])
	}
}

// boostGlyphAlpha

func TestBoostGlyphAlphaZero(t *testing.T) {
	if boostGlyphAlpha(0) != 0 {
		t.Fatal("alpha 0 should stay 0")
	}
}

func TestBoostGlyphAlpha255(t *testing.T) {
	if boostGlyphAlpha(255) != 255 {
		t.Fatal("alpha 255 should stay 255")
	}
}

func TestBoostGlyphAlphaIncreases(t *testing.T) {
	// mid-range values should be boosted (not decreased)
	for i := uint8(1); i < 255; i++ {
		if boostGlyphAlpha(i) < i {
			t.Fatalf("boostGlyphAlpha(%d) = %d < input", i, boostGlyphAlpha(i))
		}
	}
}

// fillChromeRect

func TestFillChromeRectNoPanic(t *testing.T) {
	buf := make([]byte, 100*100*4)
	fillChromeRect(buf, 100, 100, 0, 0, 50, 50)
}

func TestFillChromeRectEmptyFrame(t *testing.T) {
	buf := make([]byte, 0)
	fillChromeRect(buf, 0, 0, 0, 0, 10, 10) // must not panic
}

// chromePanel

func TestChromePanelNilPalette(t *testing.T) {
	c := chromePanel(nil)
	if c.A == 0 {
		t.Fatal("chromePanel(nil) should return opaque color")
	}
}

func TestChromePanelWithPalette(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	c := chromePanel(&pal)
	if c.A == 0 {
		t.Fatal("chromePanel with palette returned transparent")
	}
}

// applyEdgeChrome

func TestApplyEdgeChromePanicsNot(t *testing.T) {
	buf := make([]byte, 200*200*4)
	pal := newColorPalette(&config.Config{})
	applyEdgeChrome(buf, 200, 200, 10, 30, 190, 170, 0, 0, &pal)
}

func TestApplyEdgeChromeTooSmallBuf(t *testing.T) {
	buf := make([]byte, 4) // too small for 10×10
	pal := newColorPalette(&config.Config{})
	applyEdgeChrome(buf, 10, 10, 0, 0, 10, 10, 0, 0, &pal) // must not panic
}

func TestApplyEdgeChromeBandsNoPanic(t *testing.T) {
	buf := make([]byte, 200*200*4)
	pal := newColorPalette(&config.Config{})
	applyEdgeChromeBands(buf, 200, 200, 30, 170, 0, &pal)
}

func TestMaskInnerRoundedCorners(t *testing.T) {
	const fw, fh = 100, 100
	buf := make([]byte, fw*fh*4)
	bg := color.RGBA{R: 10, G: 10, B: 10, A: 255}
	fillRect(buf, fw, 20, 20, 80, 80, bg)
	featherTerminalCornersAA(buf, fw, fh, 20, 20, 80, 80, 10)
	off := (20*fw + 20) * 4
	if buf[off] == bg.R {
		t.Fatal("corner pixel should be chrome after feather")
	}
}

func TestRoundRectCoverage(t *testing.T) {
	cases := []struct {
		px, py float64
		want   float64
	}{
		{50, 50, 1},       // center
		{20.5, 20.5, 0},   // outside TL corner
		{29.5, 29.5, 1},   // inside TL corner arc
		{39.5, 21.5, 1},   // top arm between corners
		{23.5, 21.5, 0.3}, // TL corner cutout fringe
	}
	for _, tc := range cases {
		got := roundRectCoverage(tc.px, tc.py, 20, 20, 80, 80, 10)
		if got < tc.want-0.15 || got > tc.want+0.15 {
			t.Fatalf("coverage(%v,%v)=%v want ~%v", tc.px, tc.py, got, tc.want)
		}
	}
}

func TestPaintTitleChromeCentered(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	buf := make([]byte, 400*80*4)
	pal := newColorPalette(config.Defaults())
	paintTitleChrome(buf, 400, 76, &pal, b.face, "termizard", config.TitleAlignCenter, b.ascent, true)
	// Corner pixel should be title background (away from painted glyphs).
	bg := pal.bg
	off := 0
	if buf[off] != bg.R || buf[off+1] != bg.G || buf[off+2] != bg.B {
		t.Fatalf("title bg pixel = %v,%v,%v want %v", buf[off], buf[off+1], buf[off+2], bg)
	}
}

func TestPaintTitleChromeLeftAlign(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	buf := make([]byte, 800*60*4)
	pal := newColorPalette(config.Defaults())
	paintTitleChrome(buf, 800, 60, &pal, b.face, "left", config.TitleAlignLeft, b.ascent, false)
}

func TestPaintTitleChromeRightAlign(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	buf := make([]byte, 800*60*4)
	pal := newColorPalette(config.Defaults())
	paintTitleChrome(buf, 800, 60, &pal, b.face, "right", config.TitleAlignRight, b.ascent, false)
}

func TestPaintHairline(t *testing.T) {
	buf := make([]byte, 100*20*4)
	pal := newColorPalette(config.Defaults())
	paintHairline(buf, 100, 10, 11, &pal, 0.2)
}

func TestPaintInterBlockGap(t *testing.T) {
	buf := make([]byte, 200*200*4)
	paintInterBlockGap(buf, 200, 200, 50, 58)
}

func TestRenderTabBarActiveAndInactive(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	buf := make([]byte, 800*200*4)
	pal := newColorPalette(config.Defaults())
	infos := []tabBarInfo{
		{title: "~"},
		{title: "termizard"},
	}
	renderTabBar(buf, 800, 200, 40, b.cellW, b.cellH, b.ascent, 24, b.face, &pal,
		infos, 0, 1, tabDragPreview{}, 2.0, "compact", 0, 76)
}

func TestRenderTerminalGrid(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	tab := newTabSlot(12, 3, 100, false)
	tab.write([]byte("hello\r\n"))
	buf := make([]byte, 300*120*4)
	pal := newColorPalette(config.Defaults())
	rctx := renderCtx{
		pal:         &pal,
		forceAll:    true,
		showCursor:  true,
		cursorShape: cursorBlock,
	}
	if !render(tab.term, rctx, buf, 300, 120, b.cellW, b.cellH, b.ascent, 10, 12, b.face, b.fallback) {
		t.Fatal("render returned false")
	}
}

func TestRenderTabBarWithDragPreview(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	buf := make([]byte, 800*200*4)
	pal := newColorPalette(config.Defaults())
	infos := []tabBarInfo{{title: "a"}, {title: "b"}, {title: "c"}}
	drag := tabDragPreview{active: true, from: 1, over: 2, dx: 20}
	renderTabBar(buf, 800, 200, 40, b.cellW, b.cellH, b.ascent, 24, b.face, &pal,
		infos, 0, -1, drag, 2.0, "compact", 0, 76)
}

func TestRenderTabBarManyTabsOverflow(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	buf := make([]byte, 600*120*4)
	pal := newColorPalette(config.Defaults())
	infos := make([]tabBarInfo, 12)
	for i := range infos {
		infos[i] = tabBarInfo{title: fmt.Sprintf("tab%d", i)}
	}
	renderTabBar(buf, 600, 120, 36, b.cellW, b.cellH, b.ascent, 12, b.face, &pal,
		infos, 3, 5, tabDragPreview{}, 1.0, "compact", 40, 30)
}

func TestRenderWithSelection(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	tab := newTabSlot(8, 2, 100, false)
	tab.write([]byte("abcd\r\n"))
	buf := make([]byte, 200*80*4)
	pal := newColorPalette(config.Defaults())
	sel := &selectionRange{c0: 0, r0: 0, c1: 2, r1: 0}
	rctx := renderCtx{pal: &pal, forceAll: true, sel: sel, cursorShape: cursorBlock}
	render(tab.term, rctx, buf, 200, 80, b.cellW, b.cellH, b.ascent, 0, 0, b.face, b.fallback)
}

func TestApplyEdgeChrome(t *testing.T) {
	buf := make([]byte, 400*300*4)
	pal := newColorPalette(config.Defaults())
	applyEdgeChrome(buf, 400, 300, 24, 60, 376, 280, 40, 10, &pal)
}

func TestApplyEdgeChromeBands(t *testing.T) {
	buf := make([]byte, 400*300*4)
	pal := newColorPalette(config.Defaults())
	applyEdgeChromeBands(buf, 400, 300, 60, 280, 40, &pal)
}
