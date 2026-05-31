package terminal

// applySGR updates s.pen (fg/bg/attrs) from a CSI m parameter list.
// Supports: reset, bold/dim/italic/underline/blink/inverse/invisible/strikethrough,
// 16 ANSI colors, 256-indexed colors (38;5;n), 24-bit true color (38;2;r;g;b),
// and their colon-separated ISO 8613-6 equivalents.
func applySGR(s *screen, params [][]uint16) {
	if len(params) == 0 {
		s.resetPen()
		return
	}
	i := 0
	for i < len(params) {
		v := paramVal(params[i], 0)
		switch v {
		case 0:
			s.resetPen()
		case 1:
			s.attrs |= AttrBold
		case 2:
			s.attrs |= AttrDim
		case 3:
			s.attrs |= AttrItalic
		case 4:
			s.attrs |= AttrUnderline
		case 5, 6:
			s.attrs |= AttrBlink
		case 7:
			s.attrs |= AttrInverse
		case 8:
			s.attrs |= AttrInvisible
		case 9:
			s.attrs |= AttrStrikethrough
		case 22:
			s.attrs &^= AttrBold | AttrDim
		case 23:
			s.attrs &^= AttrItalic
		case 24:
			s.attrs &^= AttrUnderline
		case 25:
			s.attrs &^= AttrBlink
		case 27:
			s.attrs &^= AttrInverse
		case 28:
			s.attrs &^= AttrInvisible
		case 29:
			s.attrs &^= AttrStrikethrough
		case 30, 31, 32, 33, 34, 35, 36, 37:
			s.fg = ANSI(uint8(v - 30))
		case 38:
			c, adv := parseExtColor(params, i)
			s.fg = c
			i += adv
		case 39:
			s.fg = Color{}
		case 40, 41, 42, 43, 44, 45, 46, 47:
			s.bg = ANSI(uint8(v - 40))
		case 48:
			c, adv := parseExtColor(params, i)
			s.bg = c
			i += adv
		case 49:
			s.bg = Color{}
		case 90, 91, 92, 93, 94, 95, 96, 97:
			s.fg = ANSI(uint8(v - 90 + 8))
		case 100, 101, 102, 103, 104, 105, 106, 107:
			s.bg = ANSI(uint8(v - 100 + 8))
		}
		i++
	}
}

// parseExtColor handles 38/48 color extension in both semicolon and colon forms.
// Returns the Color and the number of additional params consumed beyond params[i].
func parseExtColor(params [][]uint16, i int) (Color, int) {
	p := params[i]

	// Colon form: sub-params inline in params[i]
	// [38, 5, n] or [38, 2, r, g, b]
	if len(p) >= 3 && p[1] == 5 {
		return Indexed(uint8(p[2])), 0
	}
	if len(p) >= 5 && p[1] == 2 {
		return RGB(uint8(p[2]), uint8(p[3]), uint8(p[4])), 0
	}

	// Semicolon form: next params are separate
	if i+1 >= len(params) {
		return Color{}, 0
	}
	switch paramVal(params[i+1], 0) {
	case 5:
		if i+2 < len(params) {
			return Indexed(uint8(paramVal(params[i+2], 0))), 2
		}
	case 2:
		if i+4 < len(params) {
			r := uint8(paramVal(params[i+2], 0))
			g := uint8(paramVal(params[i+3], 0))
			b := uint8(paramVal(params[i+4], 0))
			return RGB(r, g, b), 4
		}
	}
	return Color{}, 0
}

// paramVal returns sub-param j of p, defaulting to 0 if out of range.
func paramVal(p []uint16, j int) uint16 {
	if j < len(p) {
		return p[j]
	}
	return 0
}
