package gogpu

import (
	"fmt"

	"github.com/gogpu/gg"

	"github.com/termizard/termizard/internal/core/terminal"
)

// renderTerminal draws all cells of term into cc.
// padX/padY are logical-pixel insets from the window edge.
func (f *Gogpu) renderTerminal(cc *gg.Context, w, h int, term *terminal.Terminal, sel selSnapshot) {
	term.RLock()
	defer term.RUnlock()

	cols, rows := term.Cols(), term.Rows()
	if cols == 0 || rows == 0 || f.cellW == 0 || f.cellH == 0 {
		return
	}

	cw, ch := f.cellW, f.cellH
	padX, padY := f.padX, f.padY

	// Fill the full window with the default background colour first.
	defBG := parseHexColor(f.cfg.Colors.Background)
	cc.SetRGBA(defBG.R, defBG.G, defBG.B, 1)
	cc.DrawRectangle(0, 0, float64(w), float64(h))
	cc.Fill()

	curCol, curRow := term.CursorPos()
	selBG := parseHexColor(f.cfg.Colors.Selection)

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			cell := term.Cell(row, col)

			// Continuation half of a wide char — skip to avoid double-draw.
			if cell.Width == 0 {
				continue
			}

			x := padX + float64(col)*cw
			y := padY + float64(row)*ch
			cellW := cw * float64(cell.Width)

			fg, bg := f.resolveColors(cell)

			// Selection overrides the cell background.
			if sel.contains(row, col) {
				bg = selBG
			}

			// Always draw the cell background so glyphs from the previous frame
			// don't bleed into adjacent cells (ascenders/descenders can extend
			// outside the nominal cell rect).
			cc.SetRGBA(bg.R, bg.G, bg.B, 1)
			cc.DrawRectangle(x, y, cellW, ch)
			cc.Fill()

			// Glyph.
			if cell.Char != 0 && cell.Char != ' ' && cell.Attrs&terminal.AttrInvisible == 0 {
				if f.fontFace != nil {
					cc.SetFont(f.fontFace)
				}
				alpha := 1.0
				if cell.Attrs&terminal.AttrDim != 0 {
					alpha = 0.5
				}
				cc.SetRGBA(fg.R, fg.G, fg.B, alpha)
				// y+ascent positions the glyph on the correct baseline.
				cc.DrawString(string(cell.Char), x, y+f.ascent)
			}
		}
	}

	// Cursor — height is ascent+descent only (excludes LineGap) so the block
	// tightly wraps the glyph area instead of extending into inter-line space.
	if term.CursorVisible() {
		cx := padX + float64(curCol)*cw
		cy := padY + float64(curRow)*ch
		glyphH := f.ascent + f.descent // excludes LineGap
		cur := parseHexColor(f.cfg.Colors.Cursor)
		switch f.cfg.Cursor.Shape {
		case "beam":
			cc.SetRGBA(cur.R, cur.G, cur.B, 0.9)
			cc.DrawRectangle(cx, cy, 2, glyphH)
		case "underline":
			cc.SetRGBA(cur.R, cur.G, cur.B, 0.9)
			cc.DrawRectangle(cx, cy+glyphH-2, cw, 2)
		default: // block
			cc.SetRGBA(cur.R, cur.G, cur.B, 0.4)
			cc.DrawRectangle(cx, cy, cw, glyphH)
		}
		cc.Fill()
	}
}

// ── Colour resolution ─────────────────────────────────────────────────────────

type rgba struct{ R, G, B, A float64 }

func (f *Gogpu) resolveColors(cell terminal.Cell) (fg, bg rgba) {
	rawFG := f.resolveColor(cell.FG, true)
	rawBG := f.resolveColor(cell.BG, false)
	if cell.Attrs&terminal.AttrInverse != 0 {
		return rawBG, rawFG
	}
	return rawFG, rawBG
}

func (f *Gogpu) resolveColor(c terminal.Color, isFG bool) rgba {
	switch c.Kind {
	case terminal.ColorDefault:
		if isFG {
			return parseHexColor(f.cfg.Colors.Foreground)
		}
		return parseHexColor(f.cfg.Colors.Background)
	case terminal.ColorANSI:
		return f.ansiColor(int(c.Value))
	case terminal.ColorIndexed:
		return f.indexed256(int(c.Value))
	case terminal.ColorRGB:
		r := float64((c.Value>>16)&0xff) / 255
		g := float64((c.Value>>8)&0xff) / 255
		b := float64(c.Value&0xff) / 255
		return rgba{r, g, b, 1}
	}
	return rgba{A: 1}
}

func (f *Gogpu) ansiColor(n int) rgba {
	a := f.cfg.Colors.ANSI
	palette := [16]string{
		a.Black, a.Red, a.Green, a.Yellow,
		a.Blue, a.Magenta, a.Cyan, a.White,
		a.BrightBlack, a.BrightRed, a.BrightGreen, a.BrightYellow,
		a.BrightBlue, a.BrightMagenta, a.BrightCyan, a.BrightWhite,
	}
	if n >= 0 && n < 16 && palette[n] != "" {
		return parseHexColor(palette[n])
	}
	if n >= 0 && n < 16 {
		return defaultANSI16[n]
	}
	return rgba{A: 1}
}

func (f *Gogpu) indexed256(n int) rgba {
	if n < 16 {
		return f.ansiColor(n)
	}
	if n >= 232 {
		v := float64(8+10*(n-232)) / 255
		return rgba{v, v, v, 1}
	}
	// 6×6×6 colour cube: index = 16 + 36r + 6g + b
	idx := n - 16
	levels := [6]float64{0, 95.0 / 255, 135.0 / 255, 175.0 / 255, 215.0 / 255, 1}
	ri := idx / 36
	gi := (idx / 6) % 6
	bi := idx % 6
	return rgba{levels[ri], levels[gi], levels[bi], 1}
}

// parseHexColor parses "#RRGGBB" to rgba. Returns opaque black on error.
func parseHexColor(s string) rgba {
	if len(s) == 7 && s[0] == '#' {
		var r, g, b uint8
		if _, err := fmt.Sscanf(s[1:], "%02x%02x%02x", &r, &g, &b); err == nil {
			return rgba{float64(r) / 255, float64(g) / 255, float64(b) / 255, 1}
		}
	}
	return rgba{A: 1}
}

// defaultANSI16 is the xterm 16-colour fallback palette.
var defaultANSI16 = [16]rgba{
	{0, 0, 0, 1},             // 0  black
	{0.804, 0, 0, 1},         // 1  red
	{0, 0.804, 0, 1},         // 2  green
	{0.804, 0.804, 0, 1},     // 3  yellow
	{0, 0, 0.804, 1},         // 4  blue
	{0.804, 0, 0.804, 1},     // 5  magenta
	{0, 0.804, 0.804, 1},     // 6  cyan
	{0.898, 0.898, 0.898, 1}, // 7  white
	{0.502, 0.502, 0.502, 1}, // 8  bright black
	{1, 0.333, 0.333, 1},     // 9  bright red
	{0.333, 1, 0.333, 1},     // 10 bright green
	{1, 1, 0.333, 1},         // 11 bright yellow
	{0.333, 0.333, 1, 1},     // 12 bright blue
	{1, 0.333, 1, 1},         // 13 bright magenta
	{0.333, 1, 1, 1},         // 14 bright cyan
	{1, 1, 1, 1},             // 15 bright white
}
