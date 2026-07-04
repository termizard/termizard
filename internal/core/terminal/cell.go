package terminal

// ColorKind discriminates the four ways a terminal color can be specified.
type ColorKind uint8

const (
	ColorDefault ColorKind = iota // use the terminal's default fg/bg
	ColorANSI                     // one of the 16 named ANSI colors (0-15)
	ColorIndexed                  // 256-color palette (16-255)
	ColorRGB                      // 24-bit true color
)

// Color represents a terminal foreground or background color.
type Color struct {
	Kind  ColorKind
	Value uint32 // ANSI/indexed index, or 0x00RRGGBB for RGB
}

// ANSI constructs one of the 16 named colors (index 0-15).
func ANSI(n uint8) Color { return Color{Kind: ColorANSI, Value: uint32(n)} }

// Indexed constructs a 256-palette color (index 16-255).
func Indexed(n uint8) Color { return Color{Kind: ColorIndexed, Value: uint32(n)} }

// RGB constructs a 24-bit true-color value.
func RGB(r, g, b uint8) Color {
	return Color{Kind: ColorRGB, Value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
}

// Attrs is a bitmask of SGR text attributes.
type Attrs uint16

const (
	AttrBold          Attrs = 1 << 0
	AttrDim           Attrs = 1 << 1
	AttrItalic        Attrs = 1 << 2
	AttrUnderline     Attrs = 1 << 3
	AttrBlink         Attrs = 1 << 4
	AttrInverse       Attrs = 1 << 5
	AttrInvisible     Attrs = 1 << 6
	AttrStrikethrough Attrs = 1 << 7
)

// Cell is a single character cell on the terminal grid.
type Cell struct {
	Char  rune
	Width uint8 // 1 = narrow, 2 = wide (CJK / emoji)
	FG    Color
	BG    Color
	Attrs Attrs
}

// blankCell is the default empty cell — space, default colors, no attributes.
var blankCell = Cell{Char: ' ', Width: 1}
