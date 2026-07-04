package terminal

import "testing"

func TestScrollbackLineOutOfRange(t *testing.T) {
	term := New(10, 5, 3, true)
	if line := term.ScrollbackLine(99); line != nil {
		t.Fatalf("ScrollbackLine(99) = %v, want nil", line)
	}
}

func TestResizeNoOpSameSize(t *testing.T) {
	term := New(10, 5, 100, true)
	term.Resize(10, 5)
	if term.Cols() != 10 || term.Rows() != 5 {
		t.Fatalf("size = (%d,%d), want (10,5)", term.Cols(), term.Rows())
	}
}

func TestDCSStubsAreNoOps(t *testing.T) {
	term := New(10, 5, 100, true)
	term.DCS(nil, nil, false, 'q')
	term.DCSPut('x')
	term.DCSUnhook()
}

func TestScrollbackZeroCapacityClampsToOne(t *testing.T) {
	term := New(10, 5, 0, true)
	for i := 0; i < 5; i++ {
		term.Print('A')
		term.Execute('\n')
	}
	if term.ScrollbackLen() != 1 {
		t.Fatalf("ScrollbackLen() = %d, want 1 when capacity clamped", term.ScrollbackLen())
	}
}

func TestClampCursorOnResize(t *testing.T) {
	term := New(10, 5, 100, true)
	term.Print('X')
	term.Resize(3, 2)
	col, row := term.CursorPos()
	if col >= term.Cols() || row >= term.Rows() {
		t.Fatalf("cursor (%d,%d) out of bounds for %dx%d", col, row, term.Cols(), term.Rows())
	}
}
