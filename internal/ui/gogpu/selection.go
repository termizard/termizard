package gogpu

import (
	"strings"
	"sync"

	"github.com/gogpu/gpucontext"

	"github.com/termizard/termizard/internal/core/terminal"
)

// windowSelection holds per-window text selection state.
type windowSelection struct {
	mu        sync.RWMutex
	mouseDown bool
	active    bool
	startRow  int
	startCol  int
	endRow    int
	endCol    int
}

func (s *windowSelection) snap() selSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return selSnapshot{
		active:   s.active,
		startRow: s.startRow,
		startCol: s.startCol,
		endRow:   s.endRow,
		endCol:   s.endCol,
	}
}

func (s *windowSelection) clear() {
	s.mu.Lock()
	s.mouseDown = false
	s.active = false
	s.mu.Unlock()
}

// selSnapshot is an immutable snapshot of selection state for the render thread.
type selSnapshot struct {
	active   bool
	startRow int
	startCol int
	endRow   int
	endCol   int
}

// contains reports whether (row, col) is inside the normalised selection.
func (s selSnapshot) contains(row, col int) bool {
	if !s.active {
		return false
	}
	sr, sc, er, ec := normSel(s.startRow, s.startCol, s.endRow, s.endCol)
	if row < sr || row > er {
		return false
	}
	if row == sr && col < sc {
		return false
	}
	if row == er && col > ec {
		return false
	}
	return true
}

// normSel returns (sr,sc,er,ec) in reading order (top-left → bottom-right).
func normSel(r1, c1, r2, c2 int) (sr, sc, er, ec int) {
	if r1 < r2 || (r1 == r2 && c1 <= c2) {
		return r1, c1, r2, c2
	}
	return r2, c2, r1, c1
}

// onPointerEvent handles pointer events for text selection on the primary window.
func (f *Gogpu) onPointerEvent(ev gpucontext.PointerEvent) {
	f.handlePointerEvent(ev, f.term, &f.sel, func() { f.app.RequestRedraw() })
}

func (tw *termWindow) onPointerEvent(ev gpucontext.PointerEvent) {
	tw.f.handlePointerEvent(ev, tw.term, &tw.sel, func() { tw.f.app.RequestRedraw() })
}

func (f *Gogpu) handlePointerEvent(ev gpucontext.PointerEvent, term *terminal.Terminal, sel *windowSelection, redraw func()) {
	switch ev.Type {
	case gpucontext.PointerDown:
		if ev.Button != gpucontext.ButtonLeft {
			return
		}
		f.clearOtherSelections(sel)
		col, row := f.pixelToCell(ev.X, ev.Y, term)
		sel.mu.Lock()
		sel.mouseDown = true
		sel.active = false
		sel.startRow, sel.startCol = row, col
		sel.endRow, sel.endCol = row, col
		sel.mu.Unlock()
		redraw()

	case gpucontext.PointerMove:
		if !ev.Buttons.HasLeft() {
			return
		}
		sel.mu.RLock()
		down := sel.mouseDown
		sel.mu.RUnlock()
		if !down {
			return
		}
		col, row := f.pixelToCell(ev.X, ev.Y, term)
		sel.mu.Lock()
		sel.endRow, sel.endCol = row, col
		sel.active = sel.startRow != sel.endRow || sel.startCol != sel.endCol
		sel.mu.Unlock()
		redraw()

	case gpucontext.PointerUp:
		if ev.Button != gpucontext.ButtonLeft {
			return
		}
		sel.mu.Lock()
		sel.mouseDown = false
		active := sel.active
		sr, sc, er, ec := normSel(sel.startRow, sel.startCol, sel.endRow, sel.endCol)
		sel.mu.Unlock()
		if active {
			text := extractSelection(term, sr, sc, er, ec)
			if text != "" {
				_ = f.app.ClipboardWrite(text)
			}
		}

	case gpucontext.PointerCancel:
		sel.mu.Lock()
		sel.mouseDown = false
		sel.mu.Unlock()
	}
}

func (f *Gogpu) clearOtherSelections(active *windowSelection) {
	if active != &f.sel {
		f.sel.clear()
	}
	f.mu.Lock()
	wins := f.windows
	f.mu.Unlock()
	for _, tw := range wins {
		if active != &tw.sel {
			tw.sel.clear()
		}
	}
}

// copySelection copies the current selection of term to the clipboard.
func (f *Gogpu) copySelection(term *terminal.Terminal, sel *windowSelection) {
	snap := sel.snap()
	if !snap.active {
		return
	}
	sr, sc, er, ec := normSel(snap.startRow, snap.startCol, snap.endRow, snap.endCol)
	text := extractSelection(term, sr, sc, er, ec)
	if text != "" {
		_ = f.app.ClipboardWrite(text)
	}
}

// pixelToCell converts logical pixel coordinates to (col, row) in the terminal grid.
func (f *Gogpu) pixelToCell(x, y float64, term *terminal.Terminal) (col, row int) {
	if f.cellW == 0 || f.cellH == 0 {
		return 0, 0
	}
	col = int((x - f.padX) / f.cellW)
	row = int((y - f.padY) / f.cellH)

	term.RLock()
	maxCol := term.Cols() - 1
	maxRow := term.Rows() - 1
	term.RUnlock()

	if col < 0 {
		col = 0
	} else if col > maxCol {
		col = maxCol
	}
	if row < 0 {
		row = 0
	} else if row > maxRow {
		row = maxRow
	}
	return col, row
}

// extractSelection builds the text of the selected region.
// Trailing spaces on each line are trimmed; rows are joined with '\n'.
func extractSelection(term *terminal.Terminal, sr, sc, er, ec int) string {
	term.RLock()
	defer term.RUnlock()

	cols := term.Cols()
	var sb strings.Builder

	for row := sr; row <= er; row++ {
		startCol := 0
		endCol := cols - 1
		if row == sr {
			startCol = sc
		}
		if row == er {
			endCol = ec
		}

		// Collect runes, tracking last non-space position.
		buf := make([]rune, endCol-startCol+1)
		lastNS := -1
		for col := startCol; col <= endCol; col++ {
			cell := term.Cell(row, col)
			r := cell.Char
			if r == 0 {
				r = ' '
			}
			buf[col-startCol] = r
			if r != ' ' {
				lastNS = col - startCol
			}
		}

		if lastNS >= 0 {
			for _, r := range buf[:lastNS+1] {
				sb.WriteRune(r)
			}
		}
		if row < er {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
