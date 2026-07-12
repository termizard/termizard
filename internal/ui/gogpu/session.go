package gogpu

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gogpulib "github.com/gogpu/gogpu"
	"golang.org/x/image/font"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/core/vte"
	"github.com/termizard/termizard/internal/util/logger"
)

// tabSlot holds the terminal, parser and PTY state for one tab inside a window.
// Atomic fields are safe from any goroutine; non-atomic fields are either
// main-thread-only or guarded by the parent winState.mu.
type tabSlot struct {
	term    *terminal.Terminal
	parser  *vte.Parser
	parseMu sync.Mutex

	// PTY callbacks — set once before any goroutine access.
	keyFn      func(adapter.KeyEvent)
	resizeFn   func(adapter.ResizeEvent)
	refreshPTY func() // force SIGWINCH at current size (tmux status bar after clear)
	pokePTY    func() // refreshPTY + follow-up SIGWINCHs and redraw
	closePTY   func()

	needsPTY bool // true until onDraw spawns the shell at the final grid size

	ptyW, ptyH uint16      // last cols/rows sent to the kernel PTY (skip redundant SIGWINCH)
	ptyReady   atomic.Bool // false until spawnTabPTY returns; defers redraw from read loop

	dirty        atomic.Bool
	pendingTitle atomic.Pointer[string] // VTE → OnUpdate (main thread)
	liveTitle    atomic.Pointer[string] // OnUpdate → onDraw (render thread)
	scrollOffset atomic.Int64

	// Selection — guarded by parent winState.mu (render thread reads, main thread writes).
	selActive                bool
	selStartCol, selStartRow int
	selEndCol, selEndRow     int
}

func newTabSlot(cols, rows, scrollback int, reflow bool) *tabSlot {
	return &tabSlot{
		term:   terminal.New(cols, rows, scrollback, reflow),
		parser: vte.New(),
	}
}

// write feeds raw PTY bytes through the VTE parser and marks the slot dirty.
// Safe to call from any goroutine.
func (t *tabSlot) write(data []byte) {
	before := t.term.RepaintGen()
	t.parseMu.Lock()
	t.parser.Advance(t.term, data)
	t.parseMu.Unlock()
	t.scrollOffset.Store(0)
	t.dirty.Store(true)
	if t.term.RepaintGen() != before {
		if fn := t.pokePTY; fn != nil {
			fn()
		} else {
			refreshPTYSize(t)
		}
	}
}

// refreshPTYSize delivers SIGWINCH at the current grid size even when unchanged.
// tmux and similar programs only redraw the status bar after a resize notification.
func refreshPTYSize(tab *tabSlot) {
	if tab == nil || tab.needsPTY {
		return
	}
	if fn := tab.refreshPTY; fn != nil {
		fn()
		return
	}
	cols, rows := tab.term.Cols(), tab.term.Rows()
	if cols < 1 || rows < 1 {
		return
	}
	tab.ptyW = 0
	tab.ptyH = 0
	if fn := tab.resizeFn; fn != nil {
		togglePTYResize(fn, cols, rows)
	}
}

// togglePTYResize sends two winsize updates (r-1 then r) so tmux redraws immediately.
func togglePTYResize(fn func(adapter.ResizeEvent), cols, rows int) {
	if fn == nil {
		return
	}
	c, r := pty.ClampSize(cols, rows)
	if r > 2 {
		fn(adapter.ResizeEvent{Cols: c, Rows: r - 1})
	}
	fn(adapter.ResizeEvent{Cols: c, Rows: r})
}

// sendInput delivers bytes to the PTY.
func (t *tabSlot) sendInput(data []byte) {
	if fn := t.keyFn; fn != nil {
		logger.Get().Debug("pty input", "bytes", len(data))
		fn(adapter.KeyEvent{Data: data})
	} else {
		logger.Get().Warn("sendInput: keyFn is nil, input dropped", "bytes", len(data))
	}
}

// selNormalized returns the selection corners in top-left → bottom-right order.
// Caller must hold parent winState.mu.
func (t *tabSlot) selNormalized() (c0, r0, c1, r1 int) {
	c0, r0, c1, r1 = t.selStartCol, t.selStartRow, t.selEndCol, t.selEndRow
	if r0 > r1 || (r0 == r1 && c0 > c1) {
		c0, r0, c1, r1 = c1, r1, c0, r0
	}
	return
}

// winState holds the per-window GPU resources and the list of terminal tabs.
// GPU fields are guarded by mu; the tab slice and tab selection fields are also
// guarded by mu because onDraw (render thread) and event handlers (main thread)
// access them concurrently.
type winState struct {
	// Tabs and active index — guarded by mu.
	tabs         []*tabSlot
	activeTabIdx int

	// Window-level dirty flag: set by any PTY goroutine, used by pollDirty.
	dirty atomic.Bool

	// keyHandled suppresses the next OnTextInput event after a shortcut is consumed.
	// Accessed on the main thread only (event-loop thread).
	keyHandled bool
	// suppressLeakT drops a single leaked "t" TextInput after Cmd/Ctrl+T (macOS).
	suppressLeakT bool
	// tabHoverLock hides hover highlight until the pointer moves in the tab bar.
	tabHoverLock bool

	// Double-click detection (main thread only).
	lastClickTime time.Time
	lastClickCol  int
	lastClickRow  int
	clickCount    int

	// mouseDown tracks left-button drag for terminal selection (main thread only).
	mouseDown bool

	// Tab press/drag state (main thread only).
	tabPressActive bool
	tabPressIdx    int
	tabPressX      int // physical pixels
	tabPressY      int
	dragActive     bool
	dragTabIdx     int // index of the tab being dragged
	dragOverIdx    int // drop target index during drag
	dragDX         int // horizontal drag offset in physical pixels
	tabScrollX     int // horizontal scroll of the tab strip (physical px)

	// hoverTabIdx is the tab index under the mouse pointer (-1 = none).
	// Written on the main thread; read on the render thread (benign aligned-int race on amd64/arm64).
	hoverTabIdx int

	// Cursor blink shared across all tabs (toggled by a goroutine, read by onDraw).
	blinkOn atomic.Bool

	// Font — written by initFont (render thread); scaleFactor also read on main thread
	// (pragmatic single-word race: aligned float64 reads are atomic on amd64/arm64).
	face             font.Face
	faceFallback     font.Face // symbols missing from primary (→ ❯)
	cellW, cellH     int
	cellAscent       int
	scaleFactor      float64 // physical-pixel / logical-pixel ratio
	fontSizeCurrent  float64
	fontScaleCurrent float64

	// Render-thread-only tracking (no lock needed).
	texErrCount      int // consecutive texture allocation failures; stops redraw after 10
	lastScrollOffset int
	lastScrollGen    uint64 // terminal.ScrollGen — bumps on soft-wrap/LF scroll
	lastCurRow       int
	lastCurCol       int
	lastSelActive    bool
	lastBlinkOn      bool // last uploaded cursor-blink phase (fix cursor after scroll)
	lastTabIdx       int
	lastNumTabs      int
	lastTabTitles    []string // last painted tab labels (detect OSC title changes)
	lastTabScrollX   int
	lastHoverTabIdx  int
	lastTabBarShown  bool
	pendingGC        bool

	// GPU resources — guarded by mu.
	mu       sync.Mutex
	tex      *gogpulib.Texture
	frame    []byte
	frameAlt []byte
	frameW   int
	frameH   int
	lastCols int
	lastRows int

	layoutInProgress bool // suppress onDraw PTY resize during addNewTab spawn

	// gogpu window handle.
	win *gogpulib.Window
}

// newWinState creates a winState with a single empty tab.
func newWinState(cols, rows, scrollback int, reflow bool) (*winState, *tabSlot) {
	tab := newTabSlot(cols, rows, scrollback, reflow)
	ws := &winState{
		tabs:            []*tabSlot{tab},
		activeTabIdx:    0,
		hoverTabIdx:     -1,
		lastCurRow:      -1,
		lastCurCol:      -1,
		lastCols:        -1,
		lastRows:        -1,
		lastTabIdx:      -1,
		lastNumTabs:     -1,
		lastHoverTabIdx: -1,
	}
	ws.blinkOn.Store(true)
	return ws, tab
}

// shellTab returns the tab wired to app.New's primary PTY (always index 0).
// Output from that PTY must not follow activeTabIdx — otherwise SIGWINCH redraw
// from tab 0 lands on whichever tab is focused (duplicate prompt on first Cmd+T).
func (ws *winState) shellTab() *tabSlot {
	if len(ws.tabs) > 0 {
		return ws.tabs[0]
	}
	return nil
}

// activeTab returns the current tab. Must be called with mu held when accessed
// from the render thread; safe without mu from the main thread (sole modifier).
func (ws *winState) activeTab() *tabSlot {
	if ws.activeTabIdx >= 0 && ws.activeTabIdx < len(ws.tabs) {
		return ws.tabs[ws.activeTabIdx]
	}
	if len(ws.tabs) > 0 {
		return ws.tabs[0]
	}
	return nil
}

// extractSelectedText returns the plain-text content of the active selection.
// Trailing spaces on each row are trimmed. Returns "" when there is no selection.
func (ws *winState) extractSelectedText() string {
	ws.mu.Lock()
	entry := ws.activeTab()
	if entry == nil || !entry.selActive {
		ws.mu.Unlock()
		return ""
	}
	c0, r0, c1, r1 := entry.selNormalized()
	ws.mu.Unlock()

	entry.parseMu.Lock()
	defer entry.parseMu.Unlock()

	cols := entry.term.Cols()
	var b strings.Builder
	for row := r0; row <= r1; row++ {
		startCol := 0
		if row == r0 {
			startCol = c0
		}
		endCol := cols - 1
		if row == r1 {
			endCol = c1
		}
		lastNonSpace := startCol - 1
		for col := endCol; col >= startCol; col-- {
			cell := entry.term.Cell(row, col)
			if cell.Char != 0 && cell.Char != ' ' {
				lastNonSpace = col
				break
			}
		}
		for col := startCol; col <= lastNonSpace; col++ {
			cell := entry.term.Cell(row, col)
			if cell.Width == 0 {
				continue
			}
			if cell.Char == 0 || cell.Char == ' ' {
				b.WriteByte(' ')
			} else {
				b.WriteRune(cell.Char)
			}
		}
		if row < r1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// resetTabPointer clears in-progress tab press/drag tracking.
func (ws *winState) resetTabPointer() {
	ws.tabPressActive = false
	ws.tabPressIdx = 0
	ws.tabPressX = 0
	ws.tabPressY = 0
	ws.dragActive = false
	ws.dragTabIdx = 0
	ws.dragOverIdx = 0
	ws.dragDX = 0
}

// visualTabSlot returns the preview slot for tab index while dragging (TabBar.tsx shiftForIndex).
func visualTabSlot(index, from, over int) int {
	if from == over {
		return index
	}
	if index == from {
		return index
	}
	if from < over && index > from && index <= over {
		return index - 1
	}
	if from > over && index >= over && index < from {
		return index + 1
	}
	return index
}

// reorderTabSlots moves the tab at from to to (frontend tabs.ts semantics).
func reorderTabSlots(tabs []*tabSlot, from, to int) []*tabSlot {
	if from == to || from < 0 || to < 0 || from >= len(tabs) {
		return tabs
	}
	out := make([]*tabSlot, len(tabs))
	copy(out, tabs)
	tab := out[from]
	out = append(out[:from], out[from+1:]...)
	if to > len(out) {
		to = len(out)
	}
	out = append(out[:to], append([]*tabSlot{tab}, out[to:]...)...)
	return out
}

// activeTabPin mirrors Wails/Rio sticky-active: when overflowing and the active
// tab would be cut, it pins to the left or right of the visible strip.
func activeTabPin(activeIdx, numTabs, tabW, tabsAreaW, scrollX int) (pinLeft, pinRight bool) {
	if activeIdx < 0 || activeIdx >= numTabs || tabW <= 0 {
		return false, false
	}
	if numTabs*tabW <= tabsAreaW+2 {
		return false, false
	}
	tabLeft := activeIdx * tabW
	tabRight := tabLeft + tabW
	cutLeft := tabLeft < scrollX-1
	cutRight := tabRight > scrollX+tabsAreaW+1
	switch {
	case cutLeft && !cutRight:
		return true, false
	case cutRight && !cutLeft:
		return false, true
	case cutLeft && cutRight:
		return true, false
	}
	return false, false
}

// tabIndexAtX maps a physical X to a tab index, accounting for content inset,
// horizontal scroll and sticky pin of the active tab.
func tabIndexAtX(physX, frameW, inset, numTabs, cellW int, scaleFactor float64, size string, scrollX, activeIdx int) int {
	if numTabs <= 0 {
		return 0
	}
	tabW, _, tabsAreaW := computeTabLayout(frameW, inset, numTabs, cellW, scaleFactor, size)
	if tabW <= 0 {
		return 0
	}
	localX := physX - inset
	if localX < 0 || localX >= tabsAreaW {
		return -1
	}
	pinLeft, pinRight := activeTabPin(activeIdx, numTabs, tabW, tabsAreaW, scrollX)
	if pinLeft && localX < tabW {
		return activeIdx
	}
	if pinRight {
		pinX := tabsAreaW - tabW
		if pinX < 0 {
			pinX = 0
		}
		if localX >= pinX {
			return activeIdx
		}
	}
	contentX := localX + scrollX
	idx := contentX / tabW
	if idx >= numTabs {
		idx = numTabs - 1
	}
	if idx < 0 {
		idx = 0
	}
	// Hits that land on the scrolled (hidden) active slot under a pin still prefer pin.
	if (pinLeft || pinRight) && idx == activeIdx {
		return activeIdx
	}
	return idx
}

// shortenTabTitle keeps a readable trailing segment of a path or process title.
func shortenTabTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "~"
	}
	sep := string(filepath.Separator)
	// Normalize mixed separators so filepath.Base/Dir work on Windows titles
	// that arrive with forward slashes (or vice versa).
	var norm string
	if sep == `\` {
		norm = strings.ReplaceAll(title, `/`, `\`)
	} else {
		norm = strings.ReplaceAll(title, `\`, `/`)
	}
	if strings.Contains(norm, sep) {
		base := filepath.Base(norm)
		if base != "" && base != "." && base != sep {
			parent := filepath.Base(filepath.Dir(norm))
			if parent != "" && parent != "." && parent != sep {
				return "…" + sep + parent + sep + base
			}
			return base
		}
	}
	const maxRunes = 40
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return "…" + string(runes[len(runes)-maxRunes:])
}

// physicalFrameSize returns the framebuffer size in device pixels. When the
// platform window is not ready (0×0), falls back to the last cached size from onDraw.
func (ws *winState) physicalFrameSize() (int, int) {
	if ws.win != nil {
		if fw, fh := ws.win.PhysicalSize(); fw > 0 && fh > 0 {
			return fw, fh
		}
	}
	return ws.frameW, ws.frameH
}

func (ws *winState) hasFrameSize() bool {
	fw, fh := ws.physicalFrameSize()
	return fw > 0 && fh > 0
}

// destroyTex releases the GPU texture. Must be called with mu held.
func (ws *winState) destroyTex() {
	if ws.tex != nil {
		ws.tex.Destroy()
		ws.tex = nil
	}
}

// computeWordBounds returns the [start, end] column range of the word at (col, row).
// Word characters: alphanumeric, underscore, hyphen, dot.
func computeWordBounds(t *terminal.Terminal, col, row int) (start, end int) {
	isWord := func(ch rune) bool {
		return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.'
	}
	cols := t.Cols()
	if !isWord(t.Cell(row, col).Char) {
		return col, col
	}
	start = col
	for start > 0 && isWord(t.Cell(row, start-1).Char) {
		start--
	}
	end = col
	for end < cols-1 && isWord(t.Cell(row, end+1).Char) {
		end++
	}
	return
}

// Silence unused import of sync/atomic (struct fields keep it live).
var _ = atomic.Bool{}
