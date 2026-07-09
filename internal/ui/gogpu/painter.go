package gogpu

import (
	"fmt"
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/termizard/termizard/internal/core/terminal"
)

// selectionRange is a normalised (top-left → bottom-right) text selection.
type selectionRange struct {
	c0, r0, c1, r1 int
}

// contains reports whether (col, row) falls within the selection.
func (sr *selectionRange) contains(col, row int) bool {
	if sr == nil || row < sr.r0 || row > sr.r1 {
		return false
	}
	if row == sr.r0 && col < sr.c0 {
		return false
	}
	if row == sr.r1 && col > sr.c1 {
		return false
	}
	return true
}

// renderCtx groups the per-frame rendering parameters.
type renderCtx struct {
	pal         *colorPalette
	cursorShape string // "block", "beam", "underline"
	showCursor  bool
	scrollOff   int
	prevCurRow  int // previous cursor row, used to erase the old cursor on repaint
	forceAll    bool
	sel         *selectionRange // nil = no active selection
}

// tabBarInfo holds the display data for one tab in the software tab bar.
type tabBarInfo struct {
	title string
}

// tabDragPreview carries live drag state for tab-bar rendering (TabBar.tsx preview).
type tabDragPreview struct {
	active bool
	from   int
	over   int
	dx     int // physical pixels
}

// Tab bar rendering constants.
const (
	tabHPad          = 16   // horizontal text padding inside tab pill (physical px)
	tabVPadMin       = 2    // minimum vertical pill inset inside tab bar
	tabRadiusDiv     = 4    // tab pill corner radius = tbH / tabRadiusDiv (min 7)
	tabSepAlpha      = 0.09 // white blend strength for inactive-tab vertical divider
	tabHoverAlpha    = 0.10 // white blend strength for hovered-inactive-tab pill
	tabPinShadowBias = 0.35 // black blend strength for pinned active-tab shadow
	tabFGDimActive   = 0    // dimColor amount for active tab text (full brightness)
	tabFGDimInactive = 100  // dimColor amount for inactive tab text
	tabFGDimHovered  = 60   // dimColor amount for hovered inactive tab text (brighter)
	tabFGDimClose    = 120  // dimColor amount for close × glyph
	tabFGDimPlus     = 90   // dimColor amount for + button glyph
	tabFGDimTitle    = 60   // dimColor amount for window title text
	tabCloseExtraW   = 16   // extra width that the close button zone adds beyond cellW (padX)
)

// chromeGreen / chromeGray: Fleet diagonal chrome (olive-teal top-left → dark charcoal bottom-right).
var (
	chromeGreen = color.RGBA{R: 0x27, G: 0x38, B: 0x30, A: 255} // olive-teal (more saturated)
	chromeGray  = color.RGBA{R: 0x20, G: 0x21, B: 0x24, A: 255} // dark charcoal
)

// chromePanel is a mid-gradient solid (Clear / fallbacks). Active tab + editor use pal.bg.
func chromePanel(pal *colorPalette) color.RGBA {
	mid := blendRGBA(chromeGreen, chromeGray, 0.45)
	if pal == nil {
		return mid
	}
	return blendRGBA(pal.bg, mid, 0.55)
}

// chromeAtXY returns the Fleet chrome color at physical (x,y): greenish top-left → gray bottom-right.
func chromeAtXY(x, y, frameW, frameH int) color.RGBA {
	if frameW <= 1 || frameH <= 1 {
		return chromeGreen
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= frameW {
		x = frameW - 1
	}
	if y >= frameH {
		y = frameH - 1
	}
	nx := float64(x) / float64(frameW-1)
	ny := float64(y) / float64(frameH-1)
	// Diagonal TL→BR; slight left bias so the olive reads along the left gutter (Fleet).
	t := nx*0.52 + ny*0.48
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return blendRGBA(chromeGreen, chromeGray, t)
}

// fillChromeRect paints a rect with the diagonal green→gray chrome gradient.
func fillChromeRect(buf []byte, frameW, frameH, x0, y0, x1, y1 int) {
	if frameW <= 0 || x0 >= x1 || y0 >= y1 {
		return
	}
	if y0 < 0 {
		y0 = 0
	}
	if y1 > frameH && frameH > 0 {
		y1 = frameH
	}
	stride := frameW * 4
	for y := y0; y < y1; y++ {
		row := y * stride
		for x := x0; x < x1; x++ {
			c := chromeAtXY(x, y, frameW, frameH)
			off := row + x*4
			if off+3 >= len(buf) {
				return
			}
			buf[off] = c.R
			buf[off+1] = c.G
			buf[off+2] = c.B
			buf[off+3] = c.A
		}
	}
}

// paintTitleChrome fills the macOS FullSizeContent title underlay and draws the
// window title (centered by default); full-bleed — no side gutters on the title block.
// hasSeparator draws a 1-px hairline at the bottom only when the tab bar follows below
// (without tabs the hairline would appear as an extraneous strip).
func paintTitleChrome(buf []byte, frameW, titleH int, pal *colorPalette, face font.Face, title, align string, cellAscent int, hasSeparator bool) {
	if titleH <= 0 || frameW <= 0 || pal == nil {
		return
	}
	fillRect(buf, frameW, 0, 0, frameW, titleH, pal.bg)

	if face != nil && title != "" {
		titleFG := dimColor(pal.fg, tabFGDimTitle)
		// Measure title width.
		w := 0
		for _, ch := range title {
			if adv, ok := face.GlyphAdvance(ch); ok {
				w += adv.Ceil()
			}
		}
		var x int
		pad := titleH / 3
		if pad < 8 {
			pad = 8
		}
		switch align {
		case "left":
			x = 78 // past traffic lights (logical≈78 in CSS; scale via pad later)
			if frameW > 200 {
				x = frameW / 15
				if x < 70 {
					x = 70
				}
			}
		case "right":
			x = frameW - w - pad
		default: // center
			x = (frameW - w) / 2
		}
		if x < 0 {
			x = 0
		}
		y := (titleH + cellAscent) / 2
		if y > titleH-2 {
			y = titleH - 2
		}
		tx := x
		for _, ch := range title {
			dot := fixed.P(tx, y)
			if dr, mask, maskp, adv, ok := face.Glyph(dot, ch); ok {
				blitGlyph(buf, frameW, dr, mask, maskp, titleFG)
				tx += adv.Ceil()
			}
		}
	}

	if hasSeparator {
		paintHairline(buf, frameW, titleH-1, titleH, pal, 0.10)
	}
}

// paintHairline paints a horizontal strip blending white over base (strength 0..1).
func paintHairline(buf []byte, frameW, y0, y1 int, pal *colorPalette, strength float64) {
	if pal == nil || frameW <= 0 || y0 >= y1 {
		return
	}
	if strength <= 0 {
		strength = 0.08
	}
	line := blendRGBA(pal.bg, color.RGBA{R: 255, G: 255, B: 255, A: 255}, strength)
	fillRect(buf, frameW, 0, y0, frameW, y1, line)
}

// paintInterBlockGap fills the uniform chrome gutter between stacked blocks.
func paintInterBlockGap(buf []byte, frameW, frameH, y0, y1 int) {
	if frameW <= 0 || y0 >= y1 {
		return
	}
	fillChromeRect(buf, frameW, frameH, 0, y0, frameW, y1)
}

// renderTabBar draws a horizontal tab strip starting at yOffset.
// inset aligns the first tab and "+" with the terminal content edges (padX).
// scrollX is the physical-pixel horizontal offset of the tab strip (Rio overflow scroll).
// When overflowing, the active tab is pinned to the left or right edge (Wails/Rio sticky active).
// hoverIdx is the index of the tab currently under the mouse pointer (-1 = none).
func renderTabBar(buf []byte, frameW, frameH, tbH, cellW, cellH, cellAscent, inset int,
	face font.Face, pal *colorPalette, tabInfos []tabBarInfo, activeIdx, hoverIdx int, drag tabDragPreview, scaleFactor float64, size string, scrollX, yOffset int) {
	if tbH <= 0 || len(tabInfos) == 0 || face == nil {
		return
	}
	if frameH <= 0 {
		frameH = yOffset + tbH
	}

	fillChromeRect(buf, frameW, frameH, 0, yOffset, frameW, yOffset+tbH)

	numTabs := len(tabInfos)
	tabW, plusX, tabsAreaW := computeTabLayout(frameW, inset, numTabs, cellW, scaleFactor, size)
	if tabW <= 0 {
		return
	}
	barBG := chromeAtXY(inset+tabsAreaW/2, yOffset+tbH/2, frameW, frameH)
	origin := inset
	if origin < 0 {
		origin = 0
	}

	textTop := yOffset + (tbH-cellH)/2
	if textTop < yOffset {
		textTop = yOffset
	}

	pinLeft, pinRight := false, false
	if !drag.active {
		pinLeft, pinRight = activeTabPin(activeIdx, numTabs, tabW, tabsAreaW, scrollX)
	}

	dragIdx := -1
	if drag.active {
		dragIdx = drag.from
	}

	// Pass 0: inactive / non-pinned tabs. Pass 1: active pinned or dragged (on top).
	for pass := 0; pass < 2; pass++ {
		for i, info := range tabInfos {
			isDragged := i == dragIdx
			isActive := i == activeIdx
			isPinned := isActive && (pinLeft || pinRight)

			if pass == 0 && (isDragged || isPinned) {
				continue
			}
			if pass == 1 && !isDragged && !isPinned {
				continue
			}

			slot := i
			if drag.active {
				slot = visualTabSlot(i, drag.from, drag.over)
			}
			x := origin + slot*tabW - scrollX
			if isPinned {
				if pinLeft {
					x = origin
				} else {
					x = origin + tabsAreaW - tabW
					if x < origin {
						x = origin
					}
				}
			}
			if isDragged {
				x += drag.dx
				if x < origin {
					x = origin
				}
				if x+tabW > origin+tabsAreaW {
					x = origin + tabsAreaW - tabW
				}
				if x < origin {
					x = origin
				}
			}
			if !isDragged && !isPinned && (x+tabW <= origin || x >= origin+tabsAreaW) {
				continue
			}
			clipR := origin + tabsAreaW
			drawTabClipped(buf, frameW, tbH, yOffset, x, tabW, clipR, cellW, cellH, cellAscent, textTop,
				face, pal, barBG, info.title, i, activeIdx, hoverIdx, numTabs, isPinned)
		}
	}

	// Sticky "+" in the plus zone flush with the terminal right edge.
	contentR := frameW - inset
	plusBtnW := plusBtnWidth(cellW, scaleFactor)
	plusZoneL := contentR - plusBtnW
	if plusBtnW > 0 {
		fillChromeRect(buf, frameW, frameH, plusZoneL, yOffset, contentR, yOffset+tbH)
	}
	if plusX+cellW <= contentR && plusX >= plusZoneL {
		dot := fixed.P(plusX, textTop+cellAscent)
		if dr, mask, maskp, _, ok := face.Glyph(dot, '+'); ok {
			blitGlyph(buf, frameW, dr, mask, maskp, dimColor(pal.fg, tabFGDimPlus))
		}
	}
}

func drawTabClipped(buf []byte, frameW, tbH, yOffset, x, tabW, clipR, cellW, cellH, cellAscent, textTop int,
	face font.Face, pal *colorPalette, barBG color.RGBA, title string,
	tabIdx, activeIdx, hoverIdx, numTabs int, pinned bool) {
	if x+tabW <= 0 || x >= clipR || x >= frameW {
		return
	}

	active := tabIdx == activeIdx
	hovered := !active && tabIdx == hoverIdx
	// Inactive: muted text on chrome bar; active matches title + content (pal.bg).
	tabFG := dimColor(pal.fg, tabFGDimInactive)
	if active {
		tabFG = pal.fg
	} else if hovered {
		tabFG = dimColor(pal.fg, tabFGDimHovered)
	}

	x1 := min3(x+tabW, clipR, frameW)
	x0 := max(x, 0)
	if x0 >= x1 {
		return
	}

	// Larger pill; symmetric vertical inset inside the tab bar.
	vPad := (tbH - cellH) / 2
	if vPad < tabVPadMin {
		vPad = tabVPadMin
	}
	hPad := 2
	radius := max(7, tbH/tabRadiusDiv)
	if active {
		fillRoundRect(buf, frameW, x0+hPad, yOffset+vPad, x1-hPad, yOffset+tbH-vPad, radius, pal.bg)
		if pinned {
			shadow := color.RGBA{R: 0, G: 0, B: 0, A: 255}
			if x0 <= 1 {
				fillRect(buf, frameW, x1, yOffset+vPad, min(x1+4, clipR), yOffset+tbH-vPad, blendRGBA(barBG, shadow, tabPinShadowBias))
			} else {
				fillRect(buf, frameW, max(x0-4, 0), yOffset+vPad, x0, yOffset+tbH-vPad, blendRGBA(barBG, shadow, tabPinShadowBias))
			}
		}
	} else {
		if hovered {
			// Subtle pill highlight for the hovered inactive tab (JB Fleet style).
			hoverBG := blendRGBA(barBG, color.RGBA{R: 255, G: 255, B: 255, A: 255}, tabHoverAlpha)
			fillRoundRect(buf, frameW, x0+hPad, yOffset+vPad, x1-hPad, yOffset+tbH-vPad, radius, hoverBG)
		}
		// Thin vertical divider between inactive tabs (skip before/after active).
		sep := blendRGBA(barBG, color.RGBA{R: 255, G: 255, B: 255, A: 255}, tabSepAlpha)
		sepX := x + tabW - 1
		if tabIdx+1 != activeIdx && sepX >= 0 && sepX < clipR && sepX < frameW {
			sy0 := yOffset + vPad + 4
			sy1 := yOffset + tbH - vPad - 4
			if sy1 > sy0 {
				fillRect(buf, frameW, sepX, sy0, sepX+1, sy1, sep)
			}
		}
	}

	padX := tabHPad
	closeW := 0
	if numTabs > 1 {
		closeW = cellW + padX
	}
	if title == "" {
		title = fmt.Sprintf("Tab %d", tabIdx+1)
	}

	tx := x + padX
	for _, ch := range title {
		if tx+cellW > x+tabW-closeW-padX/2 || tx >= clipR {
			break
		}
		if tx+cellW <= 0 {
			if adv, ok := face.GlyphAdvance(ch); ok {
				tx += adv.Ceil()
			} else {
				tx += cellW
			}
			continue
		}
		dot := fixed.P(tx, textTop+cellAscent)
		if dr, mask, maskp, adv, ok := face.Glyph(dot, ch); ok {
			blitGlyph(buf, frameW, dr, mask, maskp, tabFG)
			tx += adv.Ceil()
		}
	}

	if numTabs > 1 {
		closeX := x + tabW - cellW - padX
		if closeX > x+padX && closeX+cellW <= clipR && closeX+cellW <= frameW {
			dot := fixed.P(closeX, textTop+cellAscent)
			if dr, mask, maskp, _, ok := face.Glyph(dot, '×'); ok {
				blitGlyph(buf, frameW, dr, mask, maskp, dimColor(pal.fg, tabFGDimClose))
			}
		}
	}
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// fillRoundRect fills an axis-aligned rounded rectangle (software approximating pills).
func fillRoundRect(buf []byte, frameW, x0, y0, x1, y1, radius int, c color.RGBA) {
	if x0 >= x1 || y0 >= y1 {
		return
	}
	if radius < 0 {
		radius = 0
	}
	w := x1 - x0
	h := y1 - y0
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}
	if radius <= 0 {
		fillRect(buf, frameW, x0, y0, x1, y1, c)
		return
	}
	// Middle bands + corners via distance.
	fillRect(buf, frameW, x0+radius, y0, x1-radius, y1, c)
	fillRect(buf, frameW, x0, y0+radius, x0+radius, y1-radius, c)
	fillRect(buf, frameW, x1-radius, y0+radius, x1, y1-radius, c)
	r2 := radius * radius
	paintCorner := func(cx, cy, xA, yA, xB, yB int) {
		for y := yA; y < yB; y++ {
			for x := xA; x < xB; x++ {
				dx := x - cx
				dy := y - cy
				if dx*dx+dy*dy <= r2 {
					fillRect(buf, frameW, x, y, x+1, y+1, c)
				}
			}
		}
	}
	paintCorner(x0+radius, y0+radius, x0, y0, x0+radius, y0+radius)
	paintCorner(x1-1-radius, y0+radius, x1-radius, y0, x1, y0+radius)
	paintCorner(x0+radius, y1-1-radius, x0, y1-radius, x0+radius, y1)
	paintCorner(x1-1-radius, y1-1-radius, x1-radius, y1-radius, x1, y1)
}

func blendRGBA(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t)
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

// applyEdgeChrome fills padding around the terminal grid with green→gray chrome.
// Side gutters start from chromeTop so the chrome flows continuously from the top
// (no color discontinuity between the top gutter and the side gutters).
// A 6-px bearing allowance is left on each side so col-0 / last-col ink is not wiped.
func applyEdgeChrome(buf []byte, frameW, frameH, contentL, contentT, contentR, contentB, chromeTop int, pal *colorPalette) {
	if pal == nil || frameW <= 0 || frameH <= 0 || len(buf) < frameW*frameH*4 {
		return
	}
	if contentL < 0 {
		contentL = 0
	}
	if contentT < 0 {
		contentT = 0
	}
	if contentR > frameW {
		contentR = frameW
	}
	if contentB > frameH {
		contentB = frameH
	}
	if chromeTop < 0 {
		chromeTop = 0
	}
	if chromeTop > contentT {
		chromeTop = contentT
	}

	fillChromeRect(buf, frameW, frameH, 0, chromeTop, frameW, contentT)
	fillChromeRect(buf, frameW, frameH, 0, contentB, frameW, frameH)
	// Side gutters outside glyph bearing allowance — start from chromeTop so the
	// chrome color is continuous from the top chrome band down to the bottom.
	bear := contentL
	if bear > 6 {
		bear = 6
	}
	leftWipe := contentL - bear
	if leftWipe > 0 {
		fillChromeRect(buf, frameW, frameH, 0, chromeTop, leftWipe, contentB)
	}
	rightWipe := contentR + bear
	if rightWipe < frameW {
		fillChromeRect(buf, frameW, frameH, rightWipe, chromeTop, frameW, contentB)
	}
}

// applyEdgeChromeBands refreshes only the top/bottom padding bands (safe after glyph paint).
func applyEdgeChromeBands(buf []byte, frameW, frameH, contentT, contentB, chromeTop int, pal *colorPalette) {
	if pal == nil || frameW <= 0 || frameH <= 0 {
		return
	}
	if contentT < 0 {
		contentT = 0
	}
	if contentB > frameH {
		contentB = frameH
	}
	if chromeTop < 0 {
		chromeTop = 0
	}
	if chromeTop > contentT {
		chromeTop = contentT
	}
	fillChromeRect(buf, frameW, frameH, 0, chromeTop, frameW, contentT)
	fillChromeRect(buf, frameW, frameH, 0, contentB, frameW, frameH)
}

// fillRect fills the rectangle [x0,x1) × [y0,y1) with color c in the RGBA frame buffer.
// Coordinates outside the frame are clipped (needed when a dragged tab crosses the edge).
// The first row is written pixel-by-pixel; subsequent rows use copy() for ~8x throughput.
func fillRect(buf []byte, frameW, x0, y0, x1, y1 int, c color.RGBA) {
	if frameW <= 0 || len(buf) == 0 {
		return
	}
	frameH := len(buf) / (frameW * 4)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > frameW {
		x1 = frameW
	}
	if y1 > frameH {
		y1 = frameH
	}
	if x0 >= x1 || y0 >= y1 {
		return
	}
	stride := frameW * 4
	// Fill the first row pixel-by-pixel.
	firstOff := y0*stride + x0*4
	for x := x0; x < x1; x++ {
		off := y0*stride + x*4
		buf[off] = c.R
		buf[off+1] = c.G
		buf[off+2] = c.B
		buf[off+3] = c.A
	}
	// Copy the first row into every subsequent row.
	rowBytes := (x1 - x0) * 4
	src := buf[firstOff : firstOff+rowBytes]
	for y := y0 + 1; y < y1; y++ {
		dst := buf[y*stride+x0*4:]
		copy(dst[:rowBytes], src)
	}
}

// dimColor subtracts amount from each RGB channel, clamping to zero.
func dimColor(c color.RGBA, amount uint8) color.RGBA {
	sub := func(a, b uint8) uint8 {
		if a < b {
			return 0
		}
		return a - b
	}
	return color.RGBA{R: sub(c.R, amount), G: sub(c.G, amount), B: sub(c.B, amount), A: c.A}
}

// render draws changed terminal cells into buf (RGBA pixels, stride = frameW*4).
//
// yOffset / xOffset reserve chrome (tab bar) and config padding around the grid.
// Terminal row vr is rendered starting at physical y = vr*cH + yOffset,
// column col at physical x = col*cW + xOffset.
//
// When scrollOff > 0, the top scrollOff rows are filled from the terminal scrollback
// buffer instead of the live screen. The cursor is hidden in scrollback view.
//
// Returns true when at least one pixel was updated.
func render(t *terminal.Terminal, rctx renderCtx, buf []byte, frameW, frameH, cW, cH, cAscent, yOffset, xOffset int, face, faceFallback font.Face) bool {
	cols := t.Cols()
	rows := t.Rows()
	if cols == 0 || rows == 0 || frameW <= 0 || frameH <= 0 {
		return false
	}

	curCol, curRow := t.CursorPos()
	changed := false

	for vr := 0; vr < rows; vr++ {
		y0 := vr*cH + yOffset
		if y0 >= frameH {
			break
		}

		// Determine row source.
		sbIdx := rctx.scrollOff - 1 - vr
		isSB := sbIdx >= 0
		var sbLine []terminal.Cell
		var termRow int
		if isSB {
			sbLine = t.ScrollbackLine(sbIdx) // nil if idx >= sbLen
		} else {
			termRow = vr - rctx.scrollOff
		}

		// Dirty check.
		if !rctx.forceAll {
			if isSB {
				continue
			}
			inSel := rctx.sel != nil && termRow >= rctx.sel.r0 && termRow <= rctx.sel.r1
			if !t.IsDirty(termRow) && termRow != curRow && termRow != rctx.prevCurRow && !inSel {
				continue
			}
		}

		changed = true
		y1 := y0 + cH
		if y1 > frameH {
			y1 = frameH
		}

		for col := 0; col < cols; col++ {
			x0 := col*cW + xOffset
			if x0 >= frameW {
				break
			}

			var cell terminal.Cell
			if isSB {
				if sbLine != nil && col < len(sbLine) {
					cell = sbLine[col]
				} else {
					cell.Width = 1 // blank
				}
			} else {
				cell = t.Cell(termRow, col)
			}

			isActiveCursor := rctx.showCursor && !isSB && termRow == curRow && col == curCol

			cellWCells := int(cell.Width)
			if cellWCells < 1 {
				// Wide-glyph trail / empty: still paint bg so scrolls don't leave stale ink.
				cellWCells = 1
			}
			cellPixW := cW * cellWCells
			x1 := x0 + cellPixW
			if x1 > frameW {
				x1 = frameW
			}
			// Keep ink inside the terminal inset (never into chrome / window round-clip).
			if x0 < xOffset {
				x0 = xOffset
			}
			if x1 > xOffset+cols*cW {
				x1 = xOffset + cols*cW
			}
			if x0 >= x1 {
				continue
			}

			fg := rctx.pal.resolve(cell.FG, false)
			bg := rctx.pal.resolve(cell.BG, true)

			// Block cursor: always a solid fg plate so empty cells stay visible.
			if isActiveCursor && rctx.cursorShape == cursorBlock {
				bg = rctx.pal.fg
				fg = rctx.pal.bg
			} else if cell.Attrs&terminal.AttrInverse != 0 {
				fg, bg = bg, fg
			}

			// Selection highlight: override background for selected cells
			// (unless the block cursor owns this cell).
			if !isActiveCursor && !isSB && rctx.sel.contains(col, termRow) {
				bg = rctx.pal.sel
			}

			// Fill cell background (always — including Width==0 trail cells).
			for y := y0; y < y1; y++ {
				rowOff := y * frameW * 4
				for x := x0; x < x1; x++ {
					off := rowOff + x*4
					buf[off] = bg.R
					buf[off+1] = bg.G
					buf[off+2] = bg.B
					buf[off+3] = bg.A
				}
			}

			// Trailing half of a wide glyph: no character ink.
			if cell.Width == 0 {
				if isActiveCursor {
					switch rctx.cursorShape {
					case cursorBeam:
						drawBeam(buf, frameW, x0, y0, y1, rctx.pal.fg)
					case cursorUnderline:
						drawUnderline(buf, frameW, x0, x1, y0, cH, rctx.pal.fg)
					}
				}
				continue
			}

			// Draw glyph: allow a few px of side bearing into padding, but never past
			// the window edge or into tab/title chrome.
			ch := cell.Char
			if cell.Width > 0 && ch != 0 && ch != ' ' && cell.Attrs&terminal.AttrInvisible == 0 {
				drawFace := face
				if faceFallback != nil && !faceHasRune(face, ch) && faceHasRune(faceFallback, ch) {
					drawFace = faceFallback
				}
				originX := col*cW + xOffset
				dot := fixed.P(originX, y0+cAscent)
				if dr, mask, maskp, _, ok := drawFace.Glyph(dot, ch); ok {
					clipL := originX - 3
					if clipL < 0 {
						clipL = 0
					}
					clipR := originX + cellPixW + 3
					if clipR > frameW {
						clipR = frameW
					}
					blitGlyphClipped(buf, frameW, dr, mask, maskp, fg, clipL, y0, clipR, y1)
				}
			}

			// Beam / underline on top of the glyph.
			if isActiveCursor {
				switch rctx.cursorShape {
				case cursorBeam:
					drawBeam(buf, frameW, x0, y0, y1, rctx.pal.fg)
				case cursorUnderline:
					drawUnderline(buf, frameW, x0, x1, y0, cH, rctx.pal.fg)
				}
			}
		}
	}

	return changed
}

// drawBeam draws a 1-wide vertical bar at the left edge of the cursor cell.
func drawBeam(buf []byte, frameW, x, y0, y1 int, fg color.RGBA) {
	stride := frameW * 4
	for y := y0; y < y1; y++ {
		off := y*stride + x*4
		if off+3 >= len(buf) {
			break
		}
		buf[off] = fg.R
		buf[off+1] = fg.G
		buf[off+2] = fg.B
		buf[off+3] = 255
	}
}

// drawUnderline draws a 2-pixel-tall horizontal bar at the bottom of the cursor cell.
func drawUnderline(buf []byte, frameW, x0, x1, y0, cH int, fg color.RGBA) {
	stride := frameW * 4
	thickness := 2
	barY0 := y0 + cH - thickness
	if barY0 < y0 {
		barY0 = y0
	}
	barY1 := y0 + cH
	for y := barY0; y < barY1; y++ {
		for x := x0; x < x1; x++ {
			off := y*stride + x*4
			if off+3 >= len(buf) {
				continue
			}
			buf[off] = fg.R
			buf[off+1] = fg.G
			buf[off+2] = fg.B
			buf[off+3] = 255
		}
	}
}

// boostAlphaLUT is a precomputed lookup table for boostGlyphAlpha (computed in init).
// Using a LUT avoids per-pixel integer multiplications in the glyph blit hot path.
var boostAlphaLUT [256]uint8

func init() {
	// a' = min(255, a + a*(255-a)/180) — mild S-curve without fully clipping soft edges.
	for i := range boostAlphaLUT {
		if i == 0 || i == 255 {
			boostAlphaLUT[i] = uint8(i)
			continue
		}
		v := uint32(i) + uint32(i)*uint32(255-i)/180
		if v > 255 {
			v = 255
		}
		boostAlphaLUT[i] = uint8(v)
	}
}

// boostGlyphAlpha steepens greyscale coverage so thin AA edges look denser.
func boostGlyphAlpha(a uint8) uint8 { return boostAlphaLUT[a] }

// blitGlyph composites a glyph mask onto the RGBA frame buffer (frame-bounded).
func blitGlyph(buf []byte, frameW int, dr image.Rectangle, mask image.Image, maskp image.Point, fg color.RGBA) {
	blitGlyphClipped(buf, frameW, dr, mask, maskp, fg, 0, 0, frameW, 0)
}

// blitGlyphClipped composites a glyph, discarding ink outside [cx0,cx1)×[cy0,cy1).
// Keeps side bearings from painting into chrome padding or past the terminal inset.
// cy1 <= 0 means "use full frame height".
func blitGlyphClipped(buf []byte, frameW int, dr image.Rectangle, mask image.Image, maskp image.Point, fg color.RGBA,
	cx0, cy0, cx1, cy1 int) {
	if frameW <= 0 || len(buf) == 0 {
		return
	}
	frameH := len(buf) / (frameW * 4)
	if cy1 <= 0 || cy1 > frameH {
		cy1 = frameH
	}
	if cx0 < 0 {
		cx0 = 0
	}
	if cy0 < 0 {
		cy0 = 0
	}
	if cx1 > frameW {
		cx1 = frameW
	}
	if cx0 >= cx1 || cy0 >= cy1 {
		return
	}
	alphaImg, isAlpha := mask.(*image.Alpha)
	stride := frameW * 4

	for py := dr.Min.Y; py < dr.Max.Y; py++ {
		if py < cy0 || py >= cy1 {
			continue
		}
		for px := dr.Min.X; px < dr.Max.X; px++ {
			if px < cx0 || px >= cx1 {
				continue
			}
			off := py*stride + px*4

			mx := px - dr.Min.X + maskp.X
			my := py - dr.Min.Y + maskp.Y

			var a uint8
			if isAlpha {
				a = alphaImg.AlphaAt(mx, my).A
			} else {
				_, _, _, ca := mask.At(mx, my).RGBA()
				a = uint8(ca >> 8) //nolint:gosec // G115: RGBA() returns 0-65535, >>8 gives 0-255
			}
			if a == 0 {
				continue
			}
			// Contrast boost: soft alpha → punchier stroke (closer to Kitty sharpness).
			a = boostGlyphAlpha(a)
			if a == 255 {
				buf[off] = fg.R
				buf[off+1] = fg.G
				buf[off+2] = fg.B
				buf[off+3] = 255
			} else {
				inv := uint32(255 - a)
				// Alpha-blend: (src*a + dst*(255-a))/255 is always 0-255. G115 is safe.
				buf[off] = uint8((uint32(fg.R)*uint32(a) + uint32(buf[off])*inv) / 255)     //nolint:gosec
				buf[off+1] = uint8((uint32(fg.G)*uint32(a) + uint32(buf[off+1])*inv) / 255) //nolint:gosec
				buf[off+2] = uint8((uint32(fg.B)*uint32(a) + uint32(buf[off+2])*inv) / 255) //nolint:gosec
				buf[off+3] = 255
			}
		}
	}
}
