package terminal

// Scrollback is a fixed-capacity ring buffer of terminal lines.
// Push is O(1) and allocates one []Cell copy per line.
// Line(0) returns the most recently pushed line.
type Scrollback struct {
	buf  [][]Cell
	head int // index of oldest entry
	n    int // number of lines stored
	cap  int
}

func newScrollback(capacity int) *Scrollback {
	if capacity < 1 {
		capacity = 1
	}
	return &Scrollback{buf: make([][]Cell, capacity), cap: capacity}
}

// Push copies line into the ring buffer, evicting the oldest entry when full.
func (s *Scrollback) Push(line []Cell) {
	cp := make([]Cell, len(line))
	copy(cp, line)
	if s.n < s.cap {
		s.buf[(s.head+s.n)%s.cap] = cp
		s.n++
	} else {
		s.buf[s.head] = cp
		s.head = (s.head + 1) % s.cap
	}
}

// Line returns the line at logical index idx, where 0 = most recently pushed.
// Returns nil if idx is out of range.
func (s *Scrollback) Line(idx int) []Cell {
	if idx < 0 || idx >= s.n {
		return nil
	}
	i := (s.head + s.n - 1 - idx) % s.cap
	return s.buf[i]
}

// Len returns the number of lines stored.
func (s *Scrollback) Len() int { return s.n }
