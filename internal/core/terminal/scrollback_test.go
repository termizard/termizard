package terminal

import "testing"

func lineWith(ch rune) []Cell {
	return []Cell{{Char: ch, Width: 1}}
}

func TestScrollbackEvictsOldest(t *testing.T) {
	sb := newScrollback(2)
	sb.Push(lineWith('A'))
	sb.Push(lineWith('B'))
	sb.Push(lineWith('C'))

	if sb.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", sb.Len())
	}
	if sb.Line(0)[0].Char != 'C' {
		t.Fatalf("recent line = %q, want C", sb.Line(0)[0].Char)
	}
	if sb.Line(1)[0].Char != 'B' {
		t.Fatalf("older line = %q, want B", sb.Line(1)[0].Char)
	}
	if sb.Line(2) != nil {
		t.Fatal("Line(2) should be nil")
	}
}

func TestScrollbackLineNegative(t *testing.T) {
	sb := newScrollback(2)
	if sb.Line(-1) != nil {
		t.Fatal("Line(-1) should be nil")
	}
}
