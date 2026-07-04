package terminal

import "testing"

func TestApplySGREmptyResetsPen(t *testing.T) {
	s := newScreen(10, 3)
	s.attrs = AttrBold
	s.fg = ANSI(1)
	applySGR(s, nil)
	if s.attrs != 0 || s.fg.Kind != ColorDefault {
		t.Fatal("empty SGR should reset pen")
	}
}

func TestApplySGRFullPalette(t *testing.T) {
	s := newScreen(10, 3)
	applySGR(s, [][]uint16{{5}, {6}, {8}, {39}, {49}})
	if s.attrs&(AttrBlink|AttrInvisible) == 0 {
		t.Fatal("expected blink and invisible")
	}

	applySGR(s, [][]uint16{{22}, {23}, {24}, {25}, {27}, {28}, {29}})
	if s.attrs != 0 {
		t.Fatalf("expected attrs cleared, got %v", s.attrs)
	}

	for i, code := range []uint16{30, 31, 32, 33, 34, 35, 36, 37} {
		applySGR(s, [][]uint16{{code}})
		if s.fg.Kind != ColorANSI || s.fg.Value != uint32(i) {
			t.Fatalf("fg ANSI %d: got %+v", i, s.fg)
		}
	}
	for i, code := range []uint16{40, 41, 42, 43, 44, 45, 46, 47} {
		applySGR(s, [][]uint16{{code}})
		if s.bg.Kind != ColorANSI || s.bg.Value != uint32(i) {
			t.Fatalf("bg ANSI %d: got %+v", i, s.bg)
		}
	}
	for _, code := range []uint16{90, 91, 92, 93, 94, 95, 96, 97} {
		applySGR(s, [][]uint16{{code}})
		if s.fg.Value != uint32(code-90+8) {
			t.Fatalf("bright fg %d: got %v", code, s.fg.Value)
		}
	}
	for _, code := range []uint16{100, 101, 102, 103, 104, 105, 106, 107} {
		applySGR(s, [][]uint16{{code}})
		if s.bg.Value != uint32(code-100+8) {
			t.Fatalf("bright bg %d: got %v", code, s.bg.Value)
		}
	}
}

func TestParseExtColorForms(t *testing.T) {
	s := newScreen(10, 3)

	applySGR(s, [][]uint16{{38, 5, 196}})
	if s.fg.Kind != ColorIndexed || s.fg.Value != 196 {
		t.Fatalf("indexed fg: %+v", s.fg)
	}

	applySGR(s, [][]uint16{{48, 5, 21}})
	if s.bg.Kind != ColorIndexed || s.bg.Value != 21 {
		t.Fatalf("indexed bg: %+v", s.bg)
	}

	applySGR(s, [][]uint16{{38}, {2}, {10}, {20}, {30}})
	if s.fg.Kind != ColorRGB {
		t.Fatalf("semicolon rgb fg: %+v", s.fg)
	}

	applySGR(s, [][]uint16{{48, 2, 1, 2, 3}})
	if s.bg.Kind != ColorRGB {
		t.Fatalf("inline rgb bg: %+v", s.bg)
	}
}

func TestParamValEmpty(t *testing.T) {
	if got := paramVal(nil); got != 0 {
		t.Fatalf("paramVal(nil) = %d", got)
	}
}
