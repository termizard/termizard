package terminal

// cursorState holds the cursor position and the state saved by DECSC.
type cursorState struct {
	row, col int
	visible  bool
	// saved by DECSC (ESC 7 / CSI s)
	savedRow   int
	savedCol   int
	savedFG    Color
	savedBG    Color
	savedAttrs Attrs
}

// screen is one of the two independent grids (primary / alternate).
// It owns a grid, a cursor, the SGR pen state, and the scroll region.
type screen struct {
	grid   *Grid
	cursor cursorState
	// scroll region (DECSTBM); both inclusive, 0-based
	scrollTop int
	scrollBot int
	// SGR pen
	fg    Color
	bg    Color
	attrs Attrs
	// pending wrap: cursor is at the last column and the next printable
	// character should trigger a newline before being placed.
	pendingWrap bool
}

func newScreen(cols, rows int) *screen {
	s := &screen{
		grid:      newGrid(cols, rows),
		scrollBot: rows - 1,
	}
	s.cursor.visible = true
	return s
}

func (s *screen) cols() int { return s.grid.Cols() }
func (s *screen) rows() int { return s.grid.Rows() }

func (s *screen) resetPen() {
	s.fg = Color{}
	s.bg = Color{}
	s.attrs = 0
}

// pen returns a cell template stamped with the current SGR pen.
func (s *screen) pen(r rune, w uint8) Cell {
	return Cell{Char: r, Width: w, FG: s.fg, BG: s.bg, Attrs: s.attrs}
}

// clampCursor brings the cursor inside grid bounds.
func (s *screen) clampCursor() {
	if s.cursor.row < 0 {
		s.cursor.row = 0
	} else if s.cursor.row >= s.rows() {
		s.cursor.row = s.rows() - 1
	}
	if s.cursor.col < 0 {
		s.cursor.col = 0
	} else if s.cursor.col >= s.cols() {
		s.cursor.col = s.cols() - 1
	}
}

// saveCursor saves the current cursor + pen (DECSC / ESC 7).
func (s *screen) saveCursor() {
	s.cursor.savedRow = s.cursor.row
	s.cursor.savedCol = s.cursor.col
	s.cursor.savedFG = s.fg
	s.cursor.savedBG = s.bg
	s.cursor.savedAttrs = s.attrs
}

// restoreCursor restores the previously saved cursor + pen (DECRC / ESC 8).
func (s *screen) restoreCursor() {
	s.cursor.row = s.cursor.savedRow
	s.cursor.col = s.cursor.savedCol
	s.fg = s.cursor.savedFG
	s.bg = s.cursor.savedBG
	s.attrs = s.cursor.savedAttrs
	s.clampCursor()
	s.pendingWrap = false
}

// resize returns a new screen with dimensions (cols, rows).
// Content is reflowed at the new column count: logical lines (runs of
// soft-wrapped rows) are re-broken so expanding the terminal merges orphaned
// continuation rows, and shrinking re-wraps content that no longer fits.
func (s *screen) resize(cols, rows int, reflow bool, savedLines [][]Cell, gridContinues bool) (*screen, [][]Cell, bool) {
	newGrid, newCurRow, newCurCol, saved, continues := s.grid.resize(
		cols, rows, reflow, s.cursor.row, s.cursor.col, savedLines, gridContinues,
	)
	ns := &screen{
		grid:      newGrid,
		scrollTop: 0,
		scrollBot: rows - 1,
		fg:        s.fg,
		bg:        s.bg,
		attrs:     s.attrs,
	}
	ns.cursor = s.cursor
	ns.cursor.visible = s.cursor.visible
	ns.cursor.row = newCurRow
	ns.cursor.col = newCurCol
	ns.pendingWrap = false
	return ns, saved, continues
}
