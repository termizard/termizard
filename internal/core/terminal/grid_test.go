package terminal

import "testing"

func TestCapSavedLinesEmpty(t *testing.T) {
	if got := capSavedLines(nil); got != nil {
		t.Fatalf("capSavedLines(nil) = %v", got)
	}
}

func TestCapSavedLinesDropsOldest(t *testing.T) {
	lines := make([][]Cell, 0, 6)
	for i := 0; i < 6; i++ {
		lines = append(lines, make([]Cell, 100_000))
	}
	capped := capSavedLines(lines)
	if len(capped) >= len(lines) {
		t.Fatal("expected oldest saved lines to be dropped")
	}
}
