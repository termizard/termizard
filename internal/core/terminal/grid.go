package terminal

// Grid is a two-dimensional array of Cells stored in row-major order.
// The backing store uses a stride that never shrinks on resize, so columns
// beyond the current display width are preserved rather than discarded when
// shrinking without reflow.
//
// The wrapped flag per row records whether the terminal auto-wrapped at the
// last column (DECAWM soft wrap). On resize, logical lines (runs of wrapped
// rows) are re-broken at the new column count so content that was soft-wrapped
// at a narrower width rejoins when the terminal expands — and content that fits
// at the new width appears on a single row without an orphaned continuation.
type Grid struct {
	cells   []Cell
	cols    int // logical display column count
	stride  int // backing-store width per row; >= cols, only grows
	rows    int
	dirty   []bool
	wrapped []bool // wrapped[r] = true: row r ends with an auto-wrap (soft wrap)
}

func newGrid(cols, rows int) *Grid {
	g := &Grid{
		cells:   make([]Cell, cols*rows),
		cols:    cols,
		stride:  cols,
		rows:    rows,
		dirty:   make([]bool, rows),
		wrapped: make([]bool, rows),
	}
	for i := range g.cells {
		g.cells[i] = blankCell
	}
	return g
}

func (g *Grid) Cols() int { return g.cols }
func (g *Grid) Rows() int { return g.rows }

func (g *Grid) Cell(row, col int) Cell {
	return g.cells[row*g.stride+col]
}

func (g *Grid) setCell(row, col int, c Cell) {
	g.cells[row*g.stride+col] = c
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

// markWrapped records that row r ended with an auto-wrap (pendingWrap fired).
// Called from terminal.Print before the cursor moves to the next line.
func (g *Grid) markWrapped(row int) {
	if row >= 0 && row < g.rows {
		g.wrapped[row] = true
	}
}

// clearLine fills the specified column range in one row with blank cells.
// Only the given range [fromCol, toCol] is cleared; columns outside the range
// (including overflow columns beyond the current display width) are untouched.
func (g *Grid) clearLine(row int, fromCol, toCol int) {
	base := row * g.stride
	for c := fromCol; c <= toCol; c++ {
		g.cells[base+c] = blankCell
	}
	g.dirty[row] = true
}

// clearLineFull blanks an entire row including overflow columns beyond the
// display width and resets the soft-wrap flag. Used when a row is replaced
// by a freshly-scrolled-in blank line.
func (g *Grid) clearLineFull(row int) {
	base := row * g.stride
	for c := 0; c < g.stride; c++ {
		g.cells[base+c] = blankCell
	}
	g.wrapped[row] = false
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
		copy(g.cells[row*g.stride:(row+1)*g.stride],
			g.cells[(row+n)*g.stride:(row+n+1)*g.stride])
		g.wrapped[row] = g.wrapped[row+n]
		g.dirty[row] = true
	}
	for row := bot - n + 1; row <= bot; row++ {
		g.clearLineFull(row)
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
		copy(g.cells[row*g.stride:(row+1)*g.stride],
			g.cells[(row-n)*g.stride:(row-n+1)*g.stride])
		g.wrapped[row] = g.wrapped[row-n]
		g.dirty[row] = true
	}
	for row := top; row < top+n; row++ {
		g.clearLineFull(row)
	}
}

// resize returns a new grid reflowed at the given display dimensions.
//
// Logical lines (runs of soft-wrapped visual rows followed by a non-wrapped
// row) are collected at the current g.cols width, trailing blank cells are
// trimmed, then re-broken at newCols. This means:
//   - Expanding cols merges rows that no longer need to wrap.
//   - Shrinking cols re-wraps content that no longer fits.
//
// When the reflowed content is taller than newRows, the oldest visual rows at the
// top are kept in savedLines (returned for the next resize) and optionally as
// scrollback lines so expanding the terminal width can restore them.
//
// savedLines are prepended as separate logical lines before reflow. gridContinues
// signals that the first grid logical line continues the last saved line.
// cursorRow/cursorCol indicate where the cursor is in the current grid; the
// function returns the cursor's position in the new grid.
func (g *Grid) resize(newCols, newRows, cursorRow, cursorCol int, savedLines [][]Cell, gridContinues bool) (*Grid, int, int, [][]Cell, [][]Cell, bool) {
	newStride := g.stride
	if newCols > newStride {
		newStride = newCols
	}

	// ── Phase 1: collect logical lines ───────────────────────────────────────
	// A logical line is a run of consecutive visual rows where wrapped[r]=true,
	// terminated by a row with wrapped[r]=false. Each visual row contributes
	// g.cols cells (the display-width portion only; stride overflow is excluded
	// because it may contain stale content from an earlier wider layout).

	type logLine struct {
		cells        []Cell
		cursorOffset int // cursor offset within this line (-1 if cursor elsewhere)
	}
	logicals := make([]logLine, 0, g.rows)

	row := 0
	for row < g.rows {
		startRow := row
		var cells []Cell
		for {
			base := row * g.stride
			cells = append(cells, g.cells[base:base+g.cols]...)
			isWrapped := g.wrapped[row]
			row++
			if !isWrapped || row >= g.rows {
				break
			}
		}

		// Compute cursor offset relative to this logical line's start.
		curOff := -1
		if cursorRow >= startRow && cursorRow < row {
			curOff = (cursorRow-startRow)*g.cols + cursorCol
		}

		// Trim trailing blank cells so short lines don't produce an extra
		// soft-wrap after re-breaking. Only unstyled blankCell is trimmed;
		// styled spaces (colored backgrounds etc.) are left untouched.
		end := len(cells)
		for end > 0 && cells[end-1] == blankCell {
			end--
		}

		logicals = append(logicals, logLine{cells: cells[:end], cursorOffset: curOff})
	}

	// Restore logical lines that were scrolled off during a previous narrow reflow.
	if len(savedLines) > 0 {
		restored := make([]logLine, 0, len(savedLines))
		for _, cells := range savedLines {
			restored = append(restored, logLine{cells: cells, cursorOffset: -1})
		}
		logicals = append(restored, logicals...)
	}

	// Rejoin a logical line split across saved prefix and visible grid tail.
	if gridContinues && len(savedLines) > 0 && len(logicals) > len(savedLines) {
		mergeIdx := len(savedLines) - 1
		tailIdx := len(savedLines)
		merged := append(append([]Cell(nil), logicals[mergeIdx].cells...), logicals[tailIdx].cells...)
		curOff := logicals[mergeIdx].cursorOffset
		if logicals[tailIdx].cursorOffset >= 0 {
			curOff = len(logicals[mergeIdx].cells) + logicals[tailIdx].cursorOffset
		}
		logicals[mergeIdx].cells = merged
		logicals[mergeIdx].cursorOffset = curOff
		logicals = append(logicals[:tailIdx], logicals[tailIdx+1:]...)
	}

	// ── Phase 2: compute visual row count per logical line ────────────────────

	visualRowCount := make([]int, len(logicals))
	for i, ll := range logicals {
		vr := 1
		if len(ll.cells) > newCols {
			vr = (len(ll.cells) + newCols - 1) / newCols
		}
		visualRowCount[i] = vr
	}

	// ── Phase 3: determine top-row discard offset ─────────────────────────────
	// Only content up to and including the cursor's logical line determines
	// overflow; blank trailing rows below the cursor are just empty space and
	// must not push real content off the top.

	cursorLL := len(logicals) - 1 // fallback: last line
	for i, ll := range logicals {
		if ll.cursorOffset >= 0 {
			cursorLL = i
			break
		}
	}
	contentVR := 0
	for i := 0; i <= cursorLL; i++ {
		contentVR += visualRowCount[i]
	}

	skip := 0
	if contentVR > newRows {
		skip = contentVR - newRows
	}

	// Cells in the skipped visual rows are preserved for a later expand.
	var newSavedLines [][]Cell
	var scrollbackLines [][]Cell
	if skip > 0 {
		var visualRows [][]Cell
		var rowWrapped []bool
		for i := 0; i <= cursorLL; i++ {
			rows, wrapped := splitLogicalLine(logicals[i].cells, newCols)
			visualRows = append(visualRows, rows...)
			rowWrapped = append(rowWrapped, wrapped...)
		}
		if skip > len(visualRows) {
			skip = len(visualRows)
		}
		skippedRows := visualRows[:skip]
		skippedWrapped := rowWrapped[:skip]
		newSavedLines = groupVisualRows(skippedRows, skippedWrapped)
		for _, rowCells := range skippedRows {
			line := make([]Cell, newStride)
			copy(line, rowCells)
			scrollbackLines = append(scrollbackLines, line)
		}
	}

	// ── Phase 4: build new grid ───────────────────────────────────────────────

	ng := &Grid{
		cells:   make([]Cell, newStride*newRows),
		cols:    newCols,
		stride:  newStride,
		rows:    newRows,
		dirty:   make([]bool, newRows),
		wrapped: make([]bool, newRows),
	}
	for i := range ng.cells {
		ng.cells[i] = blankCell
	}

	newCursorRow, newCursorCol := 0, 0
	newRow := 0
	startVR := 0 // cumulative visual rows before the current logical line
	newGridContinues := false

	for i, ll := range logicals {
		vr := visualRowCount[i]
		endVR := startVR + vr

		if endVR <= skip {
			// Entire logical line scrolled off the top.
			if ll.cursorOffset >= 0 {
				newCursorRow, newCursorCol = 0, 0
			}
			startVR = endVR
			continue
		}

		if newRow >= newRows {
			break
		}

		// How many visual rows at the top of this line are above the viewport?
		// (Only non-zero for the first partially-visible line.)
		rowsToSkip := 0
		if startVR < skip {
			rowsToSkip = skip - startVR
		}
		if rowsToSkip > 0 {
			newGridContinues = true
		}

		cells := ll.cells
		pos := rowsToSkip * newCols // byte offset into cells for the first visible row
		lineStartNewRow := newRow

		for newRow < newRows {
			n := newCols
			if rem := len(cells) - pos; rem < n {
				n = rem
			}
			if n > 0 {
				base := newRow * newStride
				for c := 0; c < n; c++ {
					ng.cells[base+c] = cells[pos+c]
				}
			}
			pos += n
			if pos < len(cells) {
				ng.wrapped[newRow] = true
				newRow++
			} else {
				newRow++
				break
			}
		}

		// Map cursor into the new grid.
		if ll.cursorOffset >= 0 {
			off := ll.cursorOffset
			if off > len(cells) {
				off = len(cells)
			}
			// Visual row/col within this logical line (at newCols).
			cursorVRinLine := off / newCols
			cursorVCinLine := off % newCols
			// Adjust for the portion of this line that was skipped.
			adjustedVR := cursorVRinLine - rowsToSkip
			targetRow := lineStartNewRow + adjustedVR

			switch {
			case targetRow >= 0 && targetRow < newRows:
				newCursorRow = targetRow
				newCursorCol = cursorVCinLine
			case targetRow < 0:
				// Cursor was in the skipped (scrolled-off) portion.
				newCursorRow, newCursorCol = 0, 0
			default:
				newCursorRow, newCursorCol = newRows-1, 0
			}
		}

		startVR = endVR
	}

	// Clamp into the new grid bounds.
	if newCursorRow >= newRows {
		newCursorRow = newRows - 1
	}
	if newCursorCol >= newCols {
		newCursorCol = newCols - 1
	}
	if newCursorRow < 0 {
		newCursorRow = 0
	}
	if newCursorCol < 0 {
		newCursorCol = 0
	}

	ng.markDirtyAll()
	return ng, newCursorRow, newCursorCol, newSavedLines, scrollbackLines, newGridContinues
}

// splitLogicalLine breaks a logical line into visual rows at newCols width.
// wrapped[i] is true when visual row i soft-wraps into row i+1.
func splitLogicalLine(cells []Cell, newCols int) ([][]Cell, []bool) {
	if len(cells) == 0 {
		return nil, nil
	}
	var rows [][]Cell
	var wrapped []bool
	for pos := 0; pos < len(cells); {
		n := newCols
		if rem := len(cells) - pos; rem < n {
			n = rem
		}
		rowCells := make([]Cell, n)
		copy(rowCells, cells[pos:pos+n])
		rows = append(rows, rowCells)
		pos += n
		wrapped = append(wrapped, pos < len(cells))
	}
	return rows, wrapped
}

// groupVisualRows merges consecutive soft-wrapped visual rows into logical lines.
func groupVisualRows(visualRows [][]Cell, rowWrapped []bool) [][]Cell {
	var lines [][]Cell
	for i := 0; i < len(visualRows); {
		var cells []Cell
		for {
			cells = append(cells, visualRows[i]...)
			cont := i < len(rowWrapped) && rowWrapped[i]
			i++
			if !cont || i >= len(visualRows) {
				break
			}
		}
		lines = append(lines, cells)
	}
	return lines
}

// line returns a slice of cells for the given row (used by scrollback).
// The slice spans the full backing stride, which may be wider than Cols().
func (g *Grid) line(row int) []Cell {
	return g.cells[row*g.stride : (row+1)*g.stride]
}
