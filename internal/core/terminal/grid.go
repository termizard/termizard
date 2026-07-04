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

// maxReflowSavedCells caps memory retained in reflowSavedLines across resize
// cycles. Roughly 500k cells ≈ 10 MB of Cell structs.
const maxReflowSavedCells = 500_000

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

// resize returns a new grid at the given display dimensions.
// When reflow is false, existing cells are copied without re-breaking lines.
func (g *Grid) resize(newCols, newRows int, reflow bool, cursorRow, cursorCol int, savedLines [][]Cell, gridContinues bool) (*Grid, int, int, [][]Cell, bool) {
	if !reflow {
		return g.resizeNoReflow(newCols, newRows, cursorRow, cursorCol)
	}
	return g.resizeReflow(newCols, newRows, cursorRow, cursorCol, savedLines, gridContinues)
}

// resizeNoReflow copies the overlapping region into a new grid without
// re-breaking logical lines. Overflow columns beyond the old display width
// are preserved in the backing stride.
func (g *Grid) resizeNoReflow(newCols, newRows, cursorRow, cursorCol int) (*Grid, int, int, [][]Cell, bool) {
	newStride := g.stride
	if newCols > newStride {
		newStride = newCols
	} else if newCols < newStride && newCols < g.cols {
		// Compact stride when shrinking well below the previous width.
		newStride = newCols
	}

	ng := allocGrid(newStride, newRows, newCols)
	copyRows := g.rows
	if newRows < copyRows {
		copyRows = newRows
	}
	copyCols := g.cols
	if newCols < copyCols {
		copyCols = newCols
	}
	for r := 0; r < copyRows; r++ {
		srcBase := r * g.stride
		dstBase := r * newStride
		copy(ng.cells[dstBase:dstBase+copyCols], g.cells[srcBase:srcBase+copyCols])
		if r < newRows-1 {
			ng.wrapped[r] = g.wrapped[r]
		}
	}

	newCursorRow, newCursorCol := cursorRow, cursorCol
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
	return ng, newCursorRow, newCursorCol, nil, false
}

// resizeReflow performs full column reflow (original resize logic).
func (g *Grid) resizeReflow(newCols, newRows, cursorRow, cursorCol int, savedLines [][]Cell, gridContinues bool) (*Grid, int, int, [][]Cell, bool) {
	newStride := g.stride
	if newCols > newStride {
		newStride = newCols
	} else if newCols < newStride && newCols < g.cols {
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
	// Count visual rows across all non-empty logical lines. When the total
	// exceeds newRows, scroll the oldest visual rows into reflowSavedLines so
	// a later vertical expand can restore them.

	lastNonEmpty := -1
	totalVR := 0
	for i, ll := range logicals {
		if len(ll.cells) > 0 {
			totalVR += visualRowCount[i]
			lastNonEmpty = i
		}
	}

	skip := 0
	if totalVR > newRows {
		skip = totalVR - newRows
	}

	// Skipped visual rows are preserved for a later expand via reflowSavedLines.
	var newSavedLines [][]Cell
	if skip > 0 && lastNonEmpty >= 0 {
		var visualRows [][]Cell
		var rowWrapped []bool
		for i := 0; i <= lastNonEmpty; i++ {
			rows, wrapped := splitLogicalLine(logicals[i].cells, newCols)
			visualRows = append(visualRows, rows...)
			rowWrapped = append(rowWrapped, wrapped...)
		}
		if skip > len(visualRows) {
			skip = len(visualRows)
		}
		skippedRows := visualRows[:skip]
		skippedWrapped := rowWrapped[:skip]
		newSavedLines = capSavedLines(groupVisualRows(skippedRows, skippedWrapped))
	}

	// ── Phase 4: build new grid ───────────────────────────────────────────────

	ng := allocGrid(newStride, newRows, newCols)

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
	return ng, newCursorRow, newCursorCol, newSavedLines, newGridContinues
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
		rowCells := cells[pos : pos+n]
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

// lineForScrollback returns a cols-width copy of the row for scrollback storage.
func (g *Grid) lineForScrollback(row int) []Cell {
	line := make([]Cell, g.cols)
	copy(line, g.cells[row*g.stride:row*g.stride+g.cols])
	return line
}

func allocGrid(stride, rows, cols int) *Grid {
	ng := &Grid{
		cells:   make([]Cell, stride*rows),
		cols:    cols,
		stride:  stride,
		rows:    rows,
		dirty:   make([]bool, rows),
		wrapped: make([]bool, rows),
	}
	for i := range ng.cells {
		ng.cells[i] = blankCell
	}
	return ng
}

// capSavedLines drops the oldest saved logical lines when the total cell count
// would exceed maxReflowSavedCells.
func capSavedLines(lines [][]Cell) [][]Cell {
	if len(lines) == 0 {
		return lines
	}
	total := 0
	for i := len(lines) - 1; i >= 0; i-- {
		total += len(lines[i])
		if total > maxReflowSavedCells {
			return lines[i+1:]
		}
	}
	return lines
}
