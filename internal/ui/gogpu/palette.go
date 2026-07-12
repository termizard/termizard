// Package gogpu implements the adapter.UI interface using gogpu for windowing
// and basicfont for software-rasterized terminal rendering.
package gogpu

import (
	"image/color"
	"strconv"
	"strings"

	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/util/logger"
)

// colorPalette holds the resolved RGBA colors for all terminal color slots.
type colorPalette struct {
	bg   color.RGBA
	fg   color.RGBA
	sel  color.RGBA // selection background
	ansi [16]color.RGBA
}

// newColorPalette builds a palette from config, falling back to JetBrains defaults.
func newColorPalette(cfg *config.Config) colorPalette {
	c := cfg.Colors
	p := colorPalette{
		bg:  parseColorOrDefault(c.Background, color.RGBA{R: 0x1e, G: 0x1f, B: 0x22, A: 255}),
		fg:  parseColorOrDefault(c.Foreground, color.RGBA{R: 0xBC, G: 0xBE, B: 0xC4, A: 255}),
		sel: parseColorOrDefault(c.Selection, color.RGBA{R: 0x21, G: 0x42, B: 0x83, A: 255}),
	}
	defaults := defaultANSI16()
	ansiCfg := [16]string{
		c.ANSI.Black, c.ANSI.Red, c.ANSI.Green, c.ANSI.Yellow,
		c.ANSI.Blue, c.ANSI.Magenta, c.ANSI.Cyan, c.ANSI.White,
		c.ANSI.BrightBlack, c.ANSI.BrightRed, c.ANSI.BrightGreen, c.ANSI.BrightYellow,
		c.ANSI.BrightBlue, c.ANSI.BrightMagenta, c.ANSI.BrightCyan, c.ANSI.BrightWhite,
	}
	for i := range 16 {
		p.ansi[i] = parseColorOrDefault(ansiCfg[i], defaults[i])
	}
	return p
}

// resolve maps a terminal.Color to an RGBA value using the palette.
func (p *colorPalette) resolve(c terminal.Color, isBackground bool) color.RGBA {
	switch c.Kind {
	case terminal.ColorDefault:
		if isBackground {
			return p.bg
		}
		return p.fg
	case terminal.ColorANSI:
		if int(c.Value) < 16 {
			return p.ansi[c.Value]
		}
		return p.fg
	case terminal.ColorIndexed:
		return indexed256(int(c.Value), p)
	case terminal.ColorRGB:
		return color.RGBA{
			R: uint8(c.Value >> 16), //nolint:gosec // G115: ColorRGB is 24-bit packed; shifts bound to 0-255
			G: uint8(c.Value >> 8 & 0xFF),
			B: uint8(c.Value & 0xFF),
			A: 255,
		}
	}
	return color.RGBA{A: 255}
}

// parseColorOrDefault parses a CSS color string; returns def on failure or empty input.
// Supported: #RRGGBB, #RGB, rgba(r,g,b,a). Alpha is ignored (always 255 for GPU render).
// Logs a warning when s is non-empty but unparseable so misconfigured colors are visible.
func parseColorOrDefault(s string, def color.RGBA) color.RGBA {
	if s == "" {
		return def
	}
	if c, ok := parseHexColor(s); ok {
		return c
	}
	if c, ok := parseRGBAColor(s); ok {
		return c
	}
	logger.Get().Warn("gogpu: unrecognized color in config, using default", "value", s)
	return def
}

func parseHexColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '#' {
		return color.RGBA{}, false
	}
	hex := s[1:]
	switch len(hex) {
	case 6:
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, true //nolint:gosec // G115: ParseUint with bitSize 32 bounds these shifts to ≤255
	case 3:
		v, err := strconv.ParseUint(hex, 16, 16)
		if err != nil {
			return color.RGBA{}, false
		}
		r := uint8(v>>8&0xF) * 17
		g := uint8(v>>4&0xF) * 17
		b := uint8(v&0xF) * 17
		return color.RGBA{R: r, G: g, B: b, A: 255}, true
	}
	return color.RGBA{}, false
}

// parseRGBAColor handles rgba(r,g,b,a) and rgb(r,g,b) formats.
func parseRGBAColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	var inner string
	switch {
	case strings.HasPrefix(s, "rgba(") && strings.HasSuffix(s, ")"):
		inner = s[5 : len(s)-1]
	case strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")"):
		inner = s[4 : len(s)-1]
	default:
		return color.RGBA{}, false
	}
	parts := strings.Split(inner, ",")
	if len(parts) < 3 {
		return color.RGBA{}, false
	}
	var vals [3]uint8
	for i := range 3 {
		f, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err != nil {
			return color.RGBA{}, false
		}
		// Accept 0-255 float or 0-255 int
		if f < 0 {
			f = 0
		}
		if f > 255 {
			f = 255
		}
		vals[i] = uint8(f)
	}
	return color.RGBA{R: vals[0], G: vals[1], B: vals[2], A: 255}, true
}

// indexed256 resolves a 256-color palette index (0-255) to RGBA.
// Indices 0-15 use the configured ANSI palette; 16-231 form a 6×6×6 RGB cube;
// 232-255 are 24 grayscale steps.
func indexed256(n int, p *colorPalette) color.RGBA {
	if n < 16 {
		return p.ansi[n]
	}
	if n >= 232 {
		v := uint8(8 + 10*(n-232)) //nolint:gosec // G115: n is 232-255 so 8+10*(n-232) is 8-238, fits uint8
		return color.RGBA{R: v, G: v, B: v, A: 255}
	}
	idx := n - 16
	levels := [6]uint8{0, 95, 135, 175, 215, 255}
	ri := (idx / 36) % 6
	gi := (idx / 6) % 6
	bi := idx % 6
	return color.RGBA{R: levels[ri], G: levels[gi], B: levels[bi], A: 255}
}

// defaultANSI16 returns the JetBrains IDE-inspired default 16-color palette.
func defaultANSI16() [16]color.RGBA {
	parse := func(s string) color.RGBA {
		c, _ := parseHexColor(s)
		return c
	}
	return [16]color.RGBA{
		parse("#2B2B2B"), // 0  black
		parse("#FF6B68"), // 1  red
		parse("#A8C023"), // 2  green
		parse("#FFC66D"), // 3  yellow
		parse("#5394EC"), // 4  blue
		parse("#9876AA"), // 5  magenta
		parse("#299999"), // 6  cyan
		parse("#A9B7C6"), // 7  white
		parse("#555555"), // 8  bright black
		parse("#FF8782"), // 9  bright red
		parse("#B8E986"), // 10 bright green
		parse("#FFD37F"), // 11 bright yellow
		parse("#6BAFFF"), // 12 bright blue
		parse("#C198CB"), // 13 bright magenta
		parse("#3DDAD7"), // 14 bright cyan
		parse("#FFFFFF"), // 15 bright white
	}
}
