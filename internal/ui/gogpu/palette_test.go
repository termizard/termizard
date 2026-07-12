package gogpu

import (
	"image/color"
	"testing"

	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/terminal"
)

// parseHexColor

func TestParseHexColor6Digit(t *testing.T) {
	c, ok := parseHexColor("#1e1f22")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.R != 0x1e || c.G != 0x1f || c.B != 0x22 || c.A != 255 {
		t.Fatalf("got %v", c)
	}
}

func TestParseHexColor6DigitUpper(t *testing.T) {
	c, ok := parseHexColor("#FF0080")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.R != 0xFF || c.G != 0x00 || c.B != 0x80 {
		t.Fatalf("got %v", c)
	}
}

func TestParseHexColor3Digit(t *testing.T) {
	c, ok := parseHexColor("#F0A")
	if !ok {
		t.Fatal("expected ok")
	}
	// each nibble expanded: F→FF=255, 0→00=0, A→AA=170
	if c.R != 255 || c.G != 0 || c.B != 0xAA {
		t.Fatalf("got %v", c)
	}
}

func TestParseHexColorWhitespaceTrimmed(t *testing.T) {
	_, ok := parseHexColor("  #aabbcc  ")
	if !ok {
		t.Fatal("expected ok after trim")
	}
}

func TestParseHexColorNoHash(t *testing.T) {
	_, ok := parseHexColor("aabbcc")
	if ok {
		t.Fatal("expected not ok for missing #")
	}
}

func TestParseHexColorEmpty(t *testing.T) {
	_, ok := parseHexColor("")
	if ok {
		t.Fatal("expected not ok for empty string")
	}
}

func TestParseHexColorBadLen(t *testing.T) {
	_, ok := parseHexColor("#12")
	if ok {
		t.Fatal("expected not ok for 2-digit hex")
	}
}

func TestParseHexColorInvalidChars(t *testing.T) {
	_, ok := parseHexColor("#GGHHII")
	if ok {
		t.Fatal("expected not ok for non-hex chars")
	}
}

// parseRGBAColor

func TestParseRGBAColorRGBA(t *testing.T) {
	c, ok := parseRGBAColor("rgba(255, 128, 0, 1)")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.R != 255 || c.G != 128 || c.B != 0 || c.A != 255 {
		t.Fatalf("got %v", c)
	}
}

func TestParseRGBAColorRGB(t *testing.T) {
	c, ok := parseRGBAColor("rgb(10, 20, 30)")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.R != 10 || c.G != 20 || c.B != 30 || c.A != 255 {
		t.Fatalf("got %v", c)
	}
}

func TestParseRGBAColorUppercase(t *testing.T) {
	_, ok := parseRGBAColor("RGB(1,2,3)")
	if !ok {
		t.Fatal("expected ok for uppercase RGB")
	}
}

func TestParseRGBAColorClampsAbove255(t *testing.T) {
	c, ok := parseRGBAColor("rgba(300, 0, 0, 1)")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.R != 255 {
		t.Fatalf("R not clamped: got %d", c.R)
	}
}

func TestParseRGBAColorClampsNegative(t *testing.T) {
	c, ok := parseRGBAColor("rgba(-10, 0, 0, 1)")
	if !ok {
		t.Fatal("expected ok")
	}
	if c.R != 0 {
		t.Fatalf("R not clamped to zero: got %d", c.R)
	}
}

func TestParseRGBAColorEmpty(t *testing.T) {
	_, ok := parseRGBAColor("")
	if ok {
		t.Fatal("expected not ok for empty")
	}
}

func TestParseRGBAColorInvalidPrefix(t *testing.T) {
	_, ok := parseRGBAColor("hsl(0,100%,50%)")
	if ok {
		t.Fatal("expected not ok for hsl")
	}
}

func TestParseRGBAColorTooFewParts(t *testing.T) {
	// 2 parts < 3; main assert is no panic
	_, _ = parseRGBAColor("rgb(1,2)")
}

func TestParseRGBAColorBadNumber(t *testing.T) {
	_, ok := parseRGBAColor("rgb(x,2,3)")
	if ok {
		t.Fatal("expected not ok for non-numeric")
	}
}

// parseColorOrDefault

func TestParseColorOrDefaultHex(t *testing.T) {
	def := color.RGBA{A: 255}
	c := parseColorOrDefault("#ff0000", def)
	if c.R != 255 || c.G != 0 || c.B != 0 {
		t.Fatalf("got %v", c)
	}
}

func TestParseColorOrDefaultRGBA(t *testing.T) {
	def := color.RGBA{A: 255}
	c := parseColorOrDefault("rgba(0,255,0,1)", def)
	if c.G != 255 {
		t.Fatalf("got %v", c)
	}
}

func TestParseColorOrDefaultEmpty(t *testing.T) {
	def := color.RGBA{R: 7, A: 255}
	c := parseColorOrDefault("", def)
	if c != def {
		t.Fatalf("expected default, got %v", c)
	}
}

func TestParseColorOrDefaultInvalidReturnsDefault(t *testing.T) {
	def := color.RGBA{R: 99, A: 255}
	c := parseColorOrDefault("not-a-color", def)
	if c != def {
		t.Fatalf("expected default, got %v", c)
	}
}

// indexed256

func TestIndexed256ANSIRange(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// indices 0-15 map to ANSI
	c := indexed256(0, &pal)
	if c != pal.ansi[0] {
		t.Fatalf("index 0 not ansi[0]")
	}
	c = indexed256(15, &pal)
	if c != pal.ansi[15] {
		t.Fatalf("index 15 not ansi[15]")
	}
}

func TestIndexed256GrayscaleRange(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// index 232 → v=8, all channels equal
	c := indexed256(232, &pal)
	if c.R != 8 || c.G != 8 || c.B != 8 || c.A != 255 {
		t.Fatalf("index 232 got %v", c)
	}
	// index 255 → v=8+10*23=238
	c = indexed256(255, &pal)
	if c.R != 238 {
		t.Fatalf("index 255 got %v", c)
	}
}

func TestIndexed256CubeBlack(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// index 16 → all-zero cube entry (0,0,0)
	c := indexed256(16, &pal)
	if c.R != 0 || c.G != 0 || c.B != 0 {
		t.Fatalf("cube(0,0,0) got %v", c)
	}
}

func TestIndexed256CubeWhite(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// index 231 → max cube entry (5,5,5) = (255,255,255)
	c := indexed256(231, &pal)
	if c.R != 255 || c.G != 255 || c.B != 255 {
		t.Fatalf("cube(5,5,5) got %v", c)
	}
}

func TestIndexed256CubePrimary(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// index 196 → (5,0,0) pure red
	c := indexed256(196, &pal)
	if c.R != 255 || c.G != 0 || c.B != 0 {
		t.Fatalf("cube pure red got %v", c)
	}
}

// colorPalette.resolve

func TestResolveColorDefaultBackground(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	c := pal.resolve(terminal.Color{Kind: terminal.ColorDefault}, true)
	if c != pal.bg {
		t.Fatalf("default bg: got %v want %v", c, pal.bg)
	}
}

func TestResolveColorDefaultForeground(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	c := pal.resolve(terminal.Color{Kind: terminal.ColorDefault}, false)
	if c != pal.fg {
		t.Fatalf("default fg: got %v want %v", c, pal.fg)
	}
}

func TestResolveColorANSI(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	c := pal.resolve(terminal.ANSI(1), false)
	if c != pal.ansi[1] {
		t.Fatalf("ansi[1]: got %v want %v", c, pal.ansi[1])
	}
}

func TestResolveColorANSIOutOfRange(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// value 16 is out of the 0-15 ANSI range, falls back to fg
	c := pal.resolve(terminal.Color{Kind: terminal.ColorANSI, Value: 16}, false)
	if c != pal.fg {
		t.Fatalf("out-of-range ANSI: got %v want fg %v", c, pal.fg)
	}
}

func TestResolveColorIndexed(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	c := pal.resolve(terminal.Indexed(232), false)
	// grayscale 232: R=G=B=8
	if c.R != 8 {
		t.Fatalf("indexed 232: got %v", c)
	}
}

func TestResolveColorRGB(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	c := pal.resolve(terminal.RGB(0xAB, 0xCD, 0xEF), false)
	if c.R != 0xAB || c.G != 0xCD || c.B != 0xEF || c.A != 255 {
		t.Fatalf("RGB: got %v", c)
	}
}

func TestResolveColorUnknownKind(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	// Kind=255 is not a valid ColorKind; must not panic and return opaque black
	c := pal.resolve(terminal.Color{Kind: 255}, false)
	if c.A != 255 {
		t.Fatalf("unknown kind: got %v", c)
	}
}

// newColorPalette

func TestNewColorPaletteDefaultsNonZero(t *testing.T) {
	pal := newColorPalette(&config.Config{})
	if pal.bg.A == 0 {
		t.Fatal("bg alpha should be 255")
	}
	if pal.fg.A == 0 {
		t.Fatal("fg alpha should be 255")
	}
	// 16 ANSI colors populated
	for i, ac := range pal.ansi {
		if ac.A == 0 {
			t.Fatalf("ansi[%d] alpha is 0", i)
		}
	}
}

func TestNewColorPaletteCustomColors(t *testing.T) {
	cfg := &config.Config{}
	cfg.Colors.Background = "#000000"
	cfg.Colors.Foreground = "#ffffff"
	pal := newColorPalette(cfg)
	if pal.bg.R != 0 || pal.bg.G != 0 || pal.bg.B != 0 {
		t.Fatalf("custom bg got %v", pal.bg)
	}
	if pal.fg.R != 255 || pal.fg.G != 255 || pal.fg.B != 255 {
		t.Fatalf("custom fg got %v", pal.fg)
	}
}
