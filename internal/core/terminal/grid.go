package terminal

// Grid is a two-dimensional array of Cells stored in row-major order.
// Dirty flags track which rows changed since the last renderer pass.
type Grid struct {
	cells []Cell
	cols  int
	rows  int
	dirty []bool
}

func newGrid(cols, rows int) *Grid {
	g := &Grid{
		cells: make([]Cell, cols*rows),
		cols:  cols,
		rows:  rows,
		dirty: make([]bool, rows),
	}
	for i := range g.cells {
		g.cells[i] = blankCell
	}
	return g
}

func (g *Grid) Cols() int { return g.cols }
func (g *Grid) Rows() int { return g.rows }

func (g *Grid) Cell(row, col int) Cell {
	return g.cells[row*g.cols+col]
}

func (g *Grid) setCell(row, col int, c Cell) {
	g.cells[row*g.cols+col] = c
	g.dirty[row] = true
}

// IsDirty reports whether row has been modified since the last ClearDirty call.
func (g *Grid) IsDirty(row int) bool { return g.dirty[row] }

// ClearDirty resets all dirty flags (call after each render frame).
func (g *Grid) ClearDirty() { clear(g.dirty) }

func (g *Grid) markDirtyAll() {
	for i := range g.dirty {
		g.dirty[i] = true
	}
}

// clearLine fills one row with blank cells and marks it dirty.
func (g *Grid) clearLine(row int, fromCol, toCol int) {
	base := row * g.cols
	for c := fromCol; c <= toCol; c++ {
		g.cells[base+c] = blankCell
	}
	g.dirty[row] = true
}

// scrollUp shifts rows [top, bot] up by n, blanking the bottom n rows.
// Called on linefeed when cursor is at the scroll-region bottom.
func (g *Grid) scrollUp(top, bot, n int) {
	if n <= 0 {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for row := top; row <= bot-n; row++ {
		copy(g.cells[row*g.cols:(row+1)*g.cols],
			g.cells[(row+n)*g.cols:(row+n+1)*g.cols])
		g.dirty[row] = true
	}
	for row := bot - n + 1; row <= bot; row++ {
		g.clearLine(row, 0, g.cols-1)
	}
}

// scrollDown shifts rows [top, bot] down by n, blanking the top n rows.
// Called by CSI L (insert lines) and CSI T (scroll down).
func (g *Grid) scrollDown(top, bot, n int) {
	if n <= 0 {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for row := bot; row >= top+n; row-- {
		copy(g.cells[row*g.cols:(row+1)*g.cols],
			g.cells[(row-n)*g.cols:(row-n+1)*g.cols])
		g.dirty[row] = true
	}
	for row := top; row < top+n; row++ {
		g.clearLine(row, 0, g.cols-1)
	}
}

// resize returns a new grid with dimensions (cols, rows), copying as much
// content as fits. All rows in the new grid are marked dirty.
func (g *Grid) resize(cols, rows int) *Grid {
	ng := newGrid(cols, rows)
	copyRows := min(g.rows, rows)
	copyCols := min(g.cols, cols)
	for r := 0; r < copyRows; r++ {
		for c := 0; c < copyCols; c++ {
			ng.cells[r*cols+c] = g.cells[r*g.cols+c]
		}
	}
	ng.markDirtyAll()
	return ng
}

// line returns a slice of cells for the given row (used by scrollback).
func (g *Grid) line(row int) []Cell {
	return g.cells[row*g.cols : (row+1)*g.cols]
}
