package vte

import "testing"

func TestParamsBufSemicolonAndColon(t *testing.T) {
	var b paramsBuf
	b.param('3')
	b.param('8')
	b.param(';')
	b.param('2')
	b.param(';')
	b.param('1')

	ps, _ := b.build(nil, nil)
	if len(ps) != 3 {
		t.Fatalf("params = %d, want 3", len(ps))
	}
	if ps[0][0] != 38 || ps[1][0] != 2 || ps[2][0] != 1 {
		t.Fatalf("unexpected params: %v", ps)
	}

	b.reset()
	b.param('0')
	b.param(';')
	b.param('1')
	ps, _ = b.build(nil, nil)
	if len(ps) != 2 || ps[0][0] != 0 || ps[1][0] != 1 {
		t.Fatalf("reset params = %v", ps)
	}
}

func TestParamsBufEmptyParam(t *testing.T) {
	var b paramsBuf
	b.param(';')
	b.param('1')
	ps, _ := b.build(nil, nil)
	if len(ps) != 2 {
		t.Fatalf("empty leading param: %v", ps)
	}
}
