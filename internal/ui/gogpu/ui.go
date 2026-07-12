package gogpu

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gogpulib "github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/util/logger"
)

const osDarwin = "darwin"

var isMac = runtime.GOOS == osDarwin

// isWindows enables Windows-specific rendering behavior. On Windows,
// RequestRedraw called from goroutines is silently dropped by the Win32/DXGI
// message pump; continuous rendering is used as the workaround.
var isWindows = runtime.GOOS == osWindows

const (
	defaultTitle    = "termizard"
	cursorBeam      = "beam"
	cursorUnderline = "underline"
	cursorBlock     = "block"
	actionCopy      = "Copy"
	actionPaste     = "Paste"
)

// UI implements adapter.UI using gogpu for windowing and software-rasterised terminal rendering.
// Multiple windows are supported; each window holds one or more software tabs (tabSlot).
type UI struct {
	cfg *config.Config
	app *gogpulib.App

	// Primary window — adapter.UI interface (Write, OnKeyInput, OnResize) targets its active tab.
	primary   *winState
	primaryID gogpulib.WindowID // set once in OnSurfaceAvailable

	// Pre-Run callbacks stored here, then wired to primary tab in OnSurfaceAvailable.
	keyFn    func(adapter.KeyEvent)
	resizeFn func(adapter.ResizeEvent)

	// surfaceReady is a one-shot callback deferred until the GPU surface is available.
	surfaceReady func()

	// Per-window sessions.
	sessLock sync.RWMutex
	sessions map[gogpulib.WindowID]*winState

	// Shared config (immutable after New).
	palette      colorPalette
	bindings     []compiledBinding
	cursorShape  string
	blinkEnabled bool
	baseFontSize float64

	// fontSizeDesired is shared across all windows (changed by FontIncrease/Decrease/Reset).
	fontSizeDesired float64

	// blinkOn is shared so all cursors blink in sync.
	blinkOn atomic.Bool

	// drawFrames counts total onDraw invocations (debug).
	drawFrames uint64

	// stopPoll signals background goroutines to exit.
	stopPoll chan struct{}

	// perWindowKeyPressed / perWindowTextInputed prevent EventSource from
	// double-dispatching events already handled by a per-window callback.
	// Two separate flags are required: if a single shared flag is used, the
	// KeyPress handler on Windows sets it and the following EventSource
	// OnTextInput callback sees it true and silently drops the character.
	// Each flag is consumed only by the matching EventSource event type.
	// Main-thread-only — no synchronization needed.
	perWindowKeyPressed  bool
	perWindowTextInputed bool
}

func gpuDebug() bool { return os.Getenv("TERMIZARD_GPU_DEBUG") != "" }

func (u *UI) debugf(format string, args ...any) {
	if gpuDebug() {
		fmt.Fprintf(os.Stderr, "termizard-gpu: "+format+"\n", args...)
	}
}

var _ adapter.UI = (*UI)(nil)

// New creates a UI backed by gogpu.
func New(cfg *config.Config) *UI {
	cols := cfg.Terminal.InitialCols
	rows := cfg.Terminal.InitialRows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	scrollback := cfg.Scrollback.Lines
	if scrollback <= 0 {
		scrollback = 10000
	}

	w := cfg.Window.Width
	h := cfg.Window.Height
	if w <= 0 {
		w = 1200
	}
	if h <= 0 {
		h = 800
	}
	minW := cfg.Window.MinWidth
	minH := cfg.Window.MinHeight
	if minW < 1 {
		minW = 400
	}
	if minH < 1 {
		minH = 240
	}

	title := cfg.Window.Title
	if title == "" {
		title = defaultTitle
	}

	fontSize := cfg.Font.Size
	if fontSize <= 0 {
		fontSize = 13
	}

	cursorShape := cfg.Cursor.Shape
	switch cursorShape {
	case cursorBeam, cursorUnderline:
	default:
		cursorShape = cursorBlock
	}

	appCfg := gogpulib.DefaultConfig().
		WithTitle(title).
		WithSize(w, h).
		WithMinSize(minW, minH).
		WithResizable(true).
		WithContinuousRender(isWindows). // on Windows, goroutine RequestRedraw is dropped by Win32 pump
		WithVSync(true).
		WithHeaderAlignment(headerAlignmentForConfig(cfg))

	primary, _ := newWinState(cols, rows, scrollback, cfg.Terminal.ReflowOnResize)

	u := &UI{
		cfg:             cfg,
		app:             gogpulib.NewApp(appCfg),
		primary:         primary,
		sessions:        make(map[gogpulib.WindowID]*winState),
		palette:         newColorPalette(cfg),
		bindings:        compileBindings(cfg.Keybindings),
		cursorShape:     cursorShape,
		blinkEnabled:    cfg.Cursor.Blink,
		fontSizeDesired: fontSize,
		baseFontSize:    fontSize,
	}
	u.blinkOn.Store(true)
	return u
}

// initFont loads or reloads a system monospace face (JetBrains / SF Mono / Menlo)
// with Go Mono as last resort, plus a symbol fallback for prompt arrows.
func (u *UI) initFont(ws *winState, scaleFactor float64) {
	size := u.fontSizeDesired
	if ws.face != nil && ws.fontSizeCurrent == size && ws.fontScaleCurrent == scaleFactor {
		return
	}
	b := loadFontBundle(size, scaleFactor)
	ws.face = b.face
	ws.faceFallback = b.fallback
	ws.cellW = b.cellW
	ws.cellH = b.cellH
	ws.cellAscent = b.ascent
	ws.fontSizeCurrent = size
	ws.fontScaleCurrent = scaleFactor
	ws.scaleFactor = scaleFactor
	ws.mu.Lock()
	ws.lastCols = -1
	ws.lastRows = -1
	ws.mu.Unlock()
}

// tabBarShown reports whether the tab strip is visible for numTabs.
func (u *UI) tabBarShown(numTabs int) bool {
	return u.cfg.Tabs.Enabled && (numTabs > 1 || u.cfg.Tabs.ShowWhenSingle)
}

// invalidateGridCache forces onDraw to recompute cols/rows on the next frame.
func (ws *winState) invalidateGridCache() {
	ws.lastCols = -1
	ws.lastRows = -1
}

// tabBarHeight returns the physical-pixel height of the tab bar for ws, or 0 if hidden.
// Vertical chrome above/below tab pills matches the side gutter (cellH + g).
func (u *UI) tabBarHeight(ws *winState, numTabs int) int {
	if !u.cfg.Tabs.Enabled || ws.cellH <= 0 {
		return 0
	}
	if numTabs <= 1 && !u.cfg.Tabs.ShowWhenSingle {
		return 0
	}
	g := u.termPadding(ws)
	h := ws.cellH + g
	minH := ws.cellH + 4
	if h < minH {
		h = minH
	}
	return h
}

// gridForFrame returns terminal cols/rows for the given framebuffer size and tab count.
func (u *UI) gridForFrame(ws *winState, fw, fh, numTabs int) (cols, rows int) {
	cols, rows = 80, 24
	if fw <= 0 || fh <= 0 || ws.cellW <= 0 || ws.cellH <= 0 {
		return cols, rows
	}
	bl := u.blockLayout(ws, fw, fh, numTabs)
	if bl.TermW <= 0 || bl.TermH <= 0 {
		return cols, rows
	}
	cols = bl.TermW / ws.cellW
	rows = bl.TermH / ws.cellH
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// viewportGrid returns the terminal cols/rows that fit the window for numTabs.
func (u *UI) viewportGrid(ws *winState, numTabs int) (cols, rows int) {
	fw, fh := ws.frameW, ws.frameH
	if fw <= 0 || fh <= 0 {
		fw, fh = ws.physicalFrameSize()
	}
	return u.gridForFrame(ws, fw, fh, numTabs)
}

// layoutWindowTabs resizes existing tabs when the viewport grid changes (e.g. tab bar
// show/hide). New shells are spawned from onDraw via spawnPendingPTYs so the PTY
// opens at the exact framebuffer grid — avoids a post-spawn SIGWINCH.
func (u *UI) layoutWindowTabs(ws *winState) {
	if ws == nil {
		return
	}
	fw, fh := ws.frameW, ws.frameH
	if fw <= 0 || fh <= 0 {
		fw, fh = ws.physicalFrameSize()
	}
	if fw <= 0 || fh <= 0 || ws.cellW <= 0 || ws.cellH <= 0 {
		return
	}
	cols, rows := u.gridForFrame(ws, fw, fh, len(ws.tabs))
	u.syncExistingTabsGrid(ws, cols, rows)
}

// syncExistingTabsGrid resizes tabs that already have a PTY.
func (u *UI) syncExistingTabsGrid(ws *winState, cols, rows int) {
	ws.mu.Lock()
	var existing []*tabSlot
	for _, t := range ws.tabs {
		if t != nil && !t.needsPTY {
			existing = append(existing, t)
		}
	}
	ws.mu.Unlock()
	resizeTabsToGrid(existing, cols, rows)
}

// hasPendingPTY reports whether any tab is waiting for a shell.
func (ws *winState) hasPendingPTY() bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for _, t := range ws.tabs {
		if t != nil && t.needsPTY {
			return true
		}
	}
	return false
}

// spawnPendingPTYsAtGrid starts shells for needsPTY tabs at the onDraw grid size.
func (u *UI) spawnPendingPTYsAtGrid(ws *winState, cols, rows int) {
	if ws == nil || ws.win == nil {
		return
	}
	ws.mu.Lock()
	var pending []*tabSlot
	for _, t := range ws.tabs {
		if t != nil && t.needsPTY {
			pending = append(pending, t)
		}
	}
	ws.mu.Unlock()
	if len(pending) == 0 {
		return
	}

	ws.mu.Lock()
	ws.layoutInProgress = true
	ws.lastCols = cols
	ws.lastRows = rows
	ws.mu.Unlock()
	defer func() {
		ws.mu.Lock()
		ws.layoutInProgress = false
		ws.mu.Unlock()
	}()

	winID := ws.win.ID()
	for _, tab := range pending {
		if tab.term.Cols() != cols || tab.term.Rows() != rows {
			tab.term.Resize(cols, rows)
		}
		tab.needsPTY = false
		u.spawnTabPTY(ws, tab, winID, false)
	}
}

// termPadding returns the physical-pixel padding around the terminal grid (config padding_x/y).
// Equal on all sides; on macOS FullSizeContent, bump the floor clear of the window round-rect clip.
func (u *UI) termPadding(ws *winState) int {
	sf := ws.scaleFactor
	if sf <= 0 {
		sf = 1
	}
	px := u.cfg.Window.PaddingX
	py := u.cfg.Window.PaddingY
	if px <= 0 {
		px = 10
	}
	if py <= 0 {
		py = 10
	}
	if isMac && u.cfg.Window.ShowTitleBar {
		// Rounded NSWindow clips ~10–14 logical px at the corners; keep inset above that.
		if px < 12 {
			px = 12
		}
		if py < 12 {
			py = 12
		}
	}
	// Keep X/Y equal so gutters match Fleet's even margins; return the larger value.
	if py > px {
		px = py
	}
	return int(float64(px) * sf)
}

// tabMinWidthPhys is the Wails min-width (120px) scaled to physical pixels.
func tabMinWidthPhys(scaleFactor float64) int {
	if scaleFactor <= 0 {
		scaleFactor = 1
	}
	return int(120 * scaleFactor)
}

// plusBtnWidth is the physical-pixel width reserved for the sticky "+" control.
func plusBtnWidth(cellW int, scaleFactor float64) int {
	if scaleFactor <= 0 {
		scaleFactor = 1
	}
	w := cellW + int(12*scaleFactor)
	if w < cellW+10 {
		w = cellW + 10
	}
	return w
}

// computeTabLayout returns tab slot width, sticky "+" x, and the scrollable tabs area width.
// inset is the left/right content margin (padX): first tab and "+" align with the terminal block.
func computeTabLayout(frameW, inset, numTabs, cellW int, scaleFactor float64, size string) (tabW, plusX, tabsAreaW int) {
	if numTabs <= 0 || cellW <= 0 {
		return 0, frameW - inset, frameW - 2*inset
	}
	if scaleFactor <= 0 {
		scaleFactor = 1
	}
	if inset < 0 {
		inset = 0
	}
	if inset*2 >= frameW {
		inset = 0
	}
	innerW := frameW - 2*inset
	plusBtnW := plusBtnWidth(cellW, scaleFactor)
	tabsAreaW = innerW - plusBtnW
	if tabsAreaW < cellW*2 {
		tabsAreaW = innerW
		plusBtnW = 0
	}

	compactW := int(200 * scaleFactor) // larger JB Fleet-style tabs
	minW := tabMinWidthPhys(scaleFactor)

	switch size {
	case config.TabSizeFull:
		tabW = tabsAreaW / numTabs
		if tabW < minW {
			tabW = compactW
		}
	default:
		tabW = compactW
	}

	contentR := frameW - inset
	if plusBtnW == 0 {
		plusX = contentR - cellW
		if plusX < inset {
			plusX = inset
		}
	} else {
		plusZoneL := contentR - plusBtnW
		plusX = plusZoneL + (plusBtnW-cellW)/2
		if plusX < plusZoneL {
			plusX = plusZoneL
		}
	}
	return tabW, plusX, tabsAreaW
}

func tabScrollMax(numTabs, tabW, tabsAreaW int) int {
	if numTabs <= 0 || tabW <= 0 {
		return 0
	}
	total := numTabs * tabW
	if total <= tabsAreaW {
		return 0
	}
	return total - tabsAreaW
}

func clampTabScroll(scroll, maxScroll int) int {
	if scroll < 0 {
		return 0
	}
	if scroll > maxScroll {
		return maxScroll
	}
	return scroll
}

// wireTabTitle registers OSC 0/2 title updates for a tab (path or running process).
func (u *UI) wireTabTitle(tab *tabSlot) {
	tab.term.SetOnTitle(func(title string) {
		title = normalizeOSCTitle(title)
		if title == "" {
			return
		}
		tab.pendingTitle.Store(&title)
	})
}

// OnSurfaceReady registers a one-shot callback invoked when the GPU surface is available.
func (u *UI) OnSurfaceReady(fn func()) {
	u.surfaceReady = fn
}

// Run starts the gogpu event loop. Blocks until all windows close.
// Must be called from the main goroutine.
func (u *UI) Run() error {
	u.stopPoll = make(chan struct{})
	defer close(u.stopPoll) // guaranteed exit for pollDirty/runBlink even if app.Run panics
	go u.pollDirty()
	go u.runBlink()

	u.app.OnUpdate(func(_ float64) {
		// Process pending OSC titles from all windows (main thread).
		u.sessLock.RLock()
		wins := make([]*winState, 0, len(u.sessions))
		for _, ws := range u.sessions {
			wins = append(wins, ws)
		}
		u.sessLock.RUnlock()

		for _, ws := range wins {
			ws.mu.Lock()
			tabs := make([]*tabSlot, len(ws.tabs))
			copy(tabs, ws.tabs)
			activeIdx := ws.activeTabIdx
			ws.mu.Unlock()

			for i, tab := range tabs {
				p := tab.pendingTitle.Swap(nil)
				if p == nil {
					continue
				}
				tab.liveTitle.Store(p)
				if ws == u.primary && i == activeIdx {
					// On macOS FullSizeContent we paint the title ourselves; blank
					// the native NSTextField so it doesn't show the shell OSC title.
					if isMac && u.cfg.Window.ShowTitleBar {
						u.app.SetTitle(" ")
					} else {
						u.app.SetTitle(*p)
					}
				}
				ws.dirty.Store(true)
				u.app.RequestRedraw()
			}
		}
	})

	u.app.OnResize(func(_, _ int) {
		u.app.RequestRedraw()
	})

	u.app.OnClose(func() {
		u.primary.mu.Lock()
		u.primary.destroyTex()
		u.primary.mu.Unlock()
	})

	u.app.OnAnyWindowClosed(func(id gogpulib.WindowID) {
		u.sessLock.Lock()
		ws := u.sessions[id]
		if id != u.primaryID {
			delete(u.sessions, id)
		}
		u.sessLock.Unlock()
		if ws == nil || id == u.primaryID {
			return
		}
		ws.mu.Lock()
		ws.destroyTex()
		ws.mu.Unlock()
	})

	u.app.OnSurfaceAvailable(func() {
		win := u.app.PrimaryWindow()
		if win == nil {
			return
		}

		u.primary.mu.Lock()
		primaryTab := u.primary.tabs[0]
		if u.keyFn == nil {
			logger.Get().Warn("OnSurfaceAvailable: keyFn is nil — primary tab will not send input to PTY")
		}
		primaryTab.keyFn = u.keyFn
		primaryTab.resizeFn = u.resizeFn
		primaryTab.refreshPTY = func() {
			cols, rows := primaryTab.term.Cols(), primaryTab.term.Rows()
			if cols < 1 {
				cols = 80
			}
			if rows < 1 {
				rows = 24
			}
			if u.resizeFn != nil {
				togglePTYResize(u.resizeFn, cols, rows)
			}
		}
		primaryTab.pokePTY = func() { u.pokePTYRefresh(primaryTab) }
		u.wireTabTitle(primaryTab)
		// Set an initial tab title from the configured shell name so the tab
		// shows something useful before PowerShell (or any shell) emits its
		// first OSC 0/2 sequence.
		// Initial tab title = starting cwd (matches the path inside the prompt).
		if primaryTab.pendingTitle.Load() == nil {
			if cwd := startupCwdTitle(); cwd != "" {
				primaryTab.pendingTitle.Store(&cwd)
			}
		}
		u.primary.mu.Unlock()

		u.sessLock.Lock()
		u.sessions[win.ID()] = u.primary
		u.primaryID = win.ID()
		u.sessLock.Unlock()

		u.wireWindow(win, u.primary)

		if fn := u.surfaceReady; fn != nil {
			u.surfaceReady = nil
			fn()
		}
	})

	// Use EventSource for keyboard on all platforms.
	// Per-window SetOnKeyPress/SetOnTextInput silently drop events on Linux because
	// Wayland/X11 key events have WindowID=0, causing getByPlatformID to return nil.
	// EventSource dispatches unconditionally regardless of WindowID (the Kitty pattern:
	// own the input event loop rather than delegating to a per-surface callback chain).
	src := u.app.EventSource()
	src.OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		// Skip if a per-window SetOnKeyPress callback already fired for this event.
		if u.perWindowKeyPressed {
			u.perWindowKeyPressed = false
			return
		}
		u.handleKeyPress(u.primary, key, mods)
	})
	src.OnTextInput(func(s string) {
		// Skip if a per-window SetOnTextInput callback already fired for this event.
		// NOTE: only the TextInput flag is checked here — never the KeyPress flag.
		// On Windows, SetOnKeyPress fires before WM_CHAR; using a shared flag would
		// cause EventSource to drop the following TextInput (character lost silently).
		if u.perWindowTextInputed {
			u.perWindowTextInputed = false
			return
		}
		u.handleTextInput(u.primary, s)
	})
	// On Linux, pointer and scroll events also carry WindowID=0, so per-window
	// callbacks (win.SetOnPointer/SetOnScroll) are never invoked. Route via EventSource.
	// Windows and macOS use per-window callbacks (wireWindow), so we only add EventSource
	// routing for Linux to avoid double-dispatch on those platforms.
	// OnPointer and OnScrollEvent are on sub-interfaces; type-assert to check availability.
	if runtime.GOOS == "linux" {
		if psrc, ok := src.(gpucontext.PointerEventSource); ok {
			psrc.OnPointer(func(ev gpucontext.PointerEvent) {
				u.handlePointer(u.primary, ev)
			})
		}
		if ssrc, ok := src.(gpucontext.ScrollEventSource); ok {
			ssrc.OnScrollEvent(func(ev gpucontext.ScrollEvent) {
				u.handleScroll(u.primary, ev)
			})
		}
	}

	return u.app.Run()
}

// handleTextInput processes a text-input event for ws.
// Called from the global EventSource.OnTextInput callback on all platforms.
func (u *UI) handleTextInput(ws *winState, s string) {
	logger.Get().Debug("textinput", "text", s, "keyHandled", ws.keyHandled)
	u.debugf("textinput %q keyHandled=%v", s, ws.keyHandled)
	// Suppress TextInput when a shortcut was consumed in the preceding KeyPress.
	if ws.keyHandled {
		ws.keyHandled = false
		return
	}
	// macOS may deliver a leaked "t" after Cmd/Ctrl+T even when KeyPress was handled.
	if ws.suppressLeakT && s == "t" {
		ws.suppressLeakT = false
		return
	}
	ws.suppressLeakT = false
	if s == "" {
		return
	}
	ws.mu.Lock()
	entry := ws.activeTab()
	if entry == nil {
		ws.mu.Unlock()
		return
	}
	entry.selActive = false
	ws.mu.Unlock()

	// Some platforms deliver paste as multi-rune TextInput (Wails uses ClipboardEvent).
	if len([]rune(s)) > 1 {
		u.pasteTextToTab(entry, s)
		return
	}

	entry.scrollOffset.Store(0)
	entry.sendInput([]byte(s))
}

// wireWindow attaches all per-window callbacks.
func (u *UI) wireWindow(win *gogpulib.Window, ws *winState) {
	ws.win = win

	win.SetOnDraw(func(ctx *gogpulib.Context) {
		u.onDraw(ws, ctx)
	})
	win.SetOnResize(func(_, _ int) {
		u.app.RequestRedraw()
	})
	// Keyboard input strategy:
	// Per-window callbacks are registered for all windows on all platforms.
	// On Linux (Wayland/X11), key events carry WindowID=0 so getByPlatformID
	// returns nil and dispatchKeyToWindow is silently skipped — these callbacks
	// never fire on Linux. The EventSource (registered in Run) always fires
	// unconditionally and is the actual input path on Linux.
	// On Windows/macOS, per-window callbacks fire first; each sets its own typed
	// flag so the corresponding EventSource callback skips re-dispatching the same
	// event. KeyPress and TextInput use separate flags to prevent cross-event
	// contamination (e.g. SetOnKeyPress firing before WM_CHAR must not cause
	// EventSource.OnTextInput to drop the subsequent character).
	win.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		u.perWindowKeyPressed = true
		u.handleKeyPress(ws, key, mods)
	})
	win.SetOnTextInput(func(s string) {
		u.perWindowTextInputed = true
		u.handleTextInput(ws, s)
	})
	win.SetOnScroll(func(ev gpucontext.ScrollEvent) {
		u.handleScroll(ws, ev)
	})
	win.SetOnPointer(func(ev gpucontext.PointerEvent) {
		u.handlePointer(ws, ev)
	})
	win.SetOnClose(func() bool {
		ws.mu.Lock()
		tabs := make([]*tabSlot, len(ws.tabs))
		copy(tabs, ws.tabs)
		ws.mu.Unlock()
		for _, t := range tabs {
			if t.closePTY != nil {
				t.closePTY()
			}
		}
		if win.ID() != u.primaryID {
			ws.mu.Lock()
			ws.destroyTex()
			ws.mu.Unlock()
		}
		return true
	})
}

// openNewWindow creates a new OS window with its own terminal and PTY.
// Must be called from the main thread (after Run has started).
func (u *UI) openNewWindow() {
	scrollback := u.cfg.Scrollback.Lines
	if scrollback <= 0 {
		scrollback = 10000
	}

	w, h := u.cfg.Window.Width, u.cfg.Window.Height
	if w <= 0 {
		w = 1200
	}
	if h <= 0 {
		h = 800
	}
	title := u.cfg.Window.Title
	if title == "" {
		title = defaultTitle
	}

	winCfg := gogpulib.DefaultConfig().
		WithTitle(title).
		WithSize(w, h).
		WithMinSize(400, 240).
		WithResizable(true).
		WithContinuousRender(isWindows).
		WithHeaderAlignment(headerAlignmentForConfig(u.cfg))

	win, err := u.app.NewWindow(winCfg)
	if err != nil {
		u.debugf("openNewWindow: NewWindow failed: %v", err)
		return
	}

	ws, tab := newWinState(80, 24, scrollback, u.cfg.Terminal.ReflowOnResize)

	u.sessLock.Lock()
	u.sessions[win.ID()] = ws
	u.sessLock.Unlock()

	u.wireWindow(win, ws)

	winID := win.ID()
	u.spawnTabPTY(ws, tab, winID, true)
}

// blockShortcutTextInput suppresses macOS TextInput leakage after Cmd/Ctrl+T only.
func (u *UI) blockShortcutTextInput(ws *winState) {
	ws.keyHandled = true
	ws.suppressLeakT = true
}

// addNewTab opens a new software tab in the given window.
// Must be called from the main thread.
func (u *UI) addNewTab(ws *winState) {
	scrollback := u.cfg.Scrollback.Lines
	if scrollback <= 0 {
		scrollback = 10000
	}

	ws.mu.Lock()
	numTabsAfter := len(ws.tabs) + 1
	cols, rows := u.viewportGrid(ws, numTabsAfter)
	tab := newTabSlot(cols, rows, scrollback, u.cfg.Terminal.ReflowOnResize)
	tab.needsPTY = true
	ws.tabs = append(ws.tabs, tab)
	ws.activeTabIdx = len(ws.tabs) - 1
	barAfter := u.tabBarShown(len(ws.tabs))
	if barAfter != ws.lastTabBarShown {
		ws.invalidateGridCache()
		ws.lastTabBarShown = barAfter
	}
	ws.hoverTabIdx = -1
	ws.tabHoverLock = true
	ws.mu.Unlock()

	// Shrink existing tabs when the tab bar first appears (1→2); spawn the new
	// shell on the next onDraw at the exact framebuffer grid.
	u.syncExistingTabsGrid(ws, cols, rows)
	u.app.RequestRedraw()
}

// spawnTabPTY opens a PTY for tab and starts read/wait goroutines.
// If closeWindowOnExit is true, the OS window closes when the PTY exits;
// otherwise the tab is removed from ws.
func (u *UI) spawnTabPTY(ws *winState, tab *tabSlot, winID gogpulib.WindowID, closeWindowOnExit bool) {
	env, _ := u.cfg.ShellEnvironment(nil)

	cols, rows := 80, 24
	if tab != nil {
		if c, r := tab.term.Cols(), tab.term.Rows(); c > 0 && r > 0 {
			cols, rows = c, r
		}
	}
	ptyCols, ptyRows := pty.ClampSize(cols, rows)

	cmd := u.cfg.ShellCommand()
	p, ptyErr := pty.Open(pty.Config{
		Command: cmd,
		Cols:    ptyCols,
		Rows:    ptyRows,
		Env:     env,
	})
	if ptyErr != nil {
		msg := fmt.Sprintf("\r\n\x1b[31m[termizard] shell error: %s\x1b[0m\r\n", ptyErr)
		tab.write([]byte(msg))
		ws.dirty.Store(true)
		u.app.RequestRedraw()
		return
	}

	// Show the starting cwd immediately so the tab matches the shell prompt
	// before the first OSC sequence arrives.
	if cwd := startupCwdTitle(); cwd != "" {
		tab.pendingTitle.Store(&cwd)
	}

	tab.ptyReady.Store(false)
	tab.keyFn = func(e adapter.KeyEvent) { _, _ = p.Write(e.Data) }
	tab.ptyW = ptyCols
	tab.ptyH = ptyRows
	tab.resizeFn = func(e adapter.ResizeEvent) {
		if tab.ptyW == e.Cols && tab.ptyH == e.Rows {
			return
		}
		tab.ptyW = e.Cols
		tab.ptyH = e.Rows
		_ = p.Resize(e.Cols, e.Rows)
	}
	tab.refreshPTY = func() {
		c, r := tab.term.Cols(), tab.term.Rows()
		if c < 1 || r < 1 {
			c, r = cols, rows
		}
		pc, pr := pty.ClampSize(c, r)
		if pr > 2 {
			_ = p.Resize(pc, pr-1)
		}
		_ = p.Resize(pc, pr)
	}
	tab.pokePTY = func() { u.pokePTYRefresh(tab) }
	tab.closePTY = func() { _ = p.Close() }
	u.wireTabTitle(tab)
	tab.ptyReady.Store(true)

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rdErr := p.Read(buf)
			if n > 0 {
				tab.write(buf[:n])
				if tab.ptyReady.Load() {
					ws.dirty.Store(true)
					u.app.RequestRedraw()
				}
			}
			if rdErr != nil {
				if !errors.Is(rdErr, io.EOF) {
					logger.Get().Warn("gogpu: pty read error", "err", rdErr)
				}
				break
			}
		}
	}()

	go func() {
		_ = p.Wait()
		if closeWindowOnExit {
			u.sessLock.RLock()
			_, still := u.sessions[winID]
			u.sessLock.RUnlock()
			if still && ws.win != nil {
				ws.win.Close()
			}
		} else {
			u.removeTabEntry(ws, tab)
			u.app.RequestRedraw()
		}
	}()
}

// removeTabEntry removes tab from ws.tabs and adjusts the active index.
func (u *UI) removeTabEntry(ws *winState, tab *tabSlot) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	idx := -1
	for i, t := range ws.tabs {
		if t == tab {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	ws.tabs = append(ws.tabs[:idx], ws.tabs[idx+1:]...)
	if len(ws.tabs) == 0 {
		// Window will close on its own (no more tabs); leave activeTabIdx as-is.
		return
	}
	if ws.activeTabIdx >= len(ws.tabs) {
		ws.activeTabIdx = len(ws.tabs) - 1
	}
}

// closeTabAtIndex removes the tab at idx, closing its PTY, and redraws.
func (u *UI) closeTabAtIndex(ws *winState, idx int) {
	ws.mu.Lock()
	if idx < 0 || idx >= len(ws.tabs) {
		ws.mu.Unlock()
		return
	}
	barBefore := u.tabBarShown(len(ws.tabs))
	tab := ws.tabs[idx]
	ws.tabs = append(ws.tabs[:idx], ws.tabs[idx+1:]...)
	if ws.activeTabIdx >= len(ws.tabs) {
		ws.activeTabIdx = max(0, len(ws.tabs)-1)
	}
	barAfter := u.tabBarShown(len(ws.tabs))
	if barBefore != barAfter {
		ws.invalidateGridCache()
		ws.lastTabBarShown = barAfter
	}
	ws.mu.Unlock()

	if tab.closePTY != nil {
		tab.closePTY()
	}
	if barBefore != barAfter {
		u.layoutWindowTabs(ws)
	}
	u.app.RequestRedraw()
}

// closeCurrentTab closes the active tab; if it is the last tab the window closes.
func (u *UI) closeCurrentTab(ws *winState) {
	ws.mu.Lock()
	numTabs := len(ws.tabs)
	activeIdx := ws.activeTabIdx
	ws.mu.Unlock()

	if numTabs <= 1 {
		if ws.win != nil {
			ws.win.Close()
		}
		return
	}
	u.closeTabAtIndex(ws, activeIdx)
}

// pollDirty wakes the event loop whenever any window has pending terminal output.
func (u *UI) pollDirty() {
	ticker := time.NewTicker(time.Second / 120)
	defer ticker.Stop()
	for {
		select {
		case <-u.stopPoll:
			return
		case <-ticker.C:
			dirty := false
			u.sessLock.RLock()
			for _, ws := range u.sessions {
				if ws.dirty.Load() {
					dirty = true
					break
				}
			}
			u.sessLock.RUnlock()
			if dirty {
				u.app.RequestRedraw()
			}
		}
	}
}

// runBlink toggles the shared cursor blink state at 500 ms intervals.
func (u *UI) runBlink() {
	if !u.blinkEnabled {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-u.stopPoll:
			return
		case <-ticker.C:
			u.blinkOn.Store(!u.blinkOn.Load())
			u.app.RequestRedraw()
		}
	}
}

// Close requests the application to quit and unblocks Run.
func (u *UI) Close() error {
	u.app.Quit()
	return nil
}

// OnKeyInput registers the callback for key input on the primary window's active tab.
func (u *UI) OnKeyInput(fn func(adapter.KeyEvent)) { u.keyFn = fn }

// OnResize registers the callback for terminal grid size changes on the primary window.
func (u *UI) OnResize(fn func(adapter.ResizeEvent)) { u.resizeFn = fn }

// pokePTYRefresh forces SIGWINCH (with a brief row toggle) so tmux redraws its
// status bar immediately after a full-screen clear.
func (u *UI) pokePTYRefresh(tab *tabSlot) {
	if tab == nil {
		return
	}
	refreshPTYSize(tab)
	u.app.RequestRedraw()
}

// RequestRedraw schedules a new draw frame.
func (u *UI) RequestRedraw() { u.app.RequestRedraw() }

// Write feeds raw PTY output from app.New's primary shell to tab 0.
func (u *UI) Write(data []byte) (int, error) {
	u.primary.mu.Lock()
	entry := u.primary.shellTab()
	u.primary.mu.Unlock()
	if entry != nil {
		entry.write(data)
		u.primary.dirty.Store(true)
	}
	logger.Get().Debug("pty output", "bytes", len(data))
	if gpuDebug() {
		fmt.Fprintf(os.Stderr, "termizard-gpu: write %d bytes\n", len(data))
	}
	u.app.RequestRedraw()
	return len(data), nil
}

// NotifyShellError displays a shell error in the primary window's active tab.
func (u *UI) NotifyShellError(tabID int, err error) {
	if err == nil {
		return
	}
	msg := fmt.Sprintf("\r\n\x1b[31m[termizard] shell error (tab %d): %s\x1b[0m\r\n", tabID, err.Error())
	u.primary.mu.Lock()
	entry := u.primary.shellTab()
	u.primary.mu.Unlock()
	if entry != nil {
		entry.write([]byte(msg))
		u.primary.dirty.Store(true)
	}
	u.app.RequestRedraw()
}

// ── Draw callback ─────────────────────────────────────────────────────────────

func (u *UI) onDraw(ws *winState, ctx *gogpulib.Context) {
	n := atomic.AddUint64(&u.drawFrames, 1)
	u.initFont(ws, ctx.ScaleFactor())

	fw, fh := ctx.FramebufferSize()
	if fw <= 0 || fh <= 0 {
		fw, fh = u.app.PhysicalSize()
	}
	if fw <= 0 || fh <= 0 || ws.cellW <= 0 || ws.cellH <= 0 {
		u.debugf("onDraw #%d skip: fw=%d fh=%d cell=%dx%d", n, fw, fh, ws.cellW, ws.cellH)
		if n == 1 {
			logger.Get().Warn("first onDraw skipped", "fw", fw, "fh", fh, "cellW", ws.cellW, "cellH", ws.cellH)
		}
		u.app.RequestRedraw()
		return
	}

	// Snapshot tab state.
	ws.mu.Lock()
	numTabs := len(ws.tabs)
	activeIdx := ws.activeTabIdx
	if activeIdx >= numTabs {
		activeIdx = max(0, numTabs-1)
	}
	var tabInfos []tabBarInfo
	tbH := u.tabBarHeight(ws, numTabs)
	titlesChanged := false
	if tbH > 0 {
		tabInfos = make([]tabBarInfo, numTabs)
		labelMode := u.cfg.Tabs.Label
		for i, t := range ws.tabs {
			title := fmt.Sprintf("Tab %d", i+1)
			if labelMode != config.TabLabelIndex {
				tp := t.liveTitle.Load()
				if tp != nil && *tp != "" {
					title = shortenTabTitle(*tp)
				} else {
					title = "~"
				}
			}
			tabInfos[i] = tabBarInfo{title: title}
		}
		if len(ws.lastTabTitles) != len(tabInfos) {
			titlesChanged = true
		} else {
			for i := range tabInfos {
				if ws.lastTabTitles[i] != tabInfos[i].title {
					titlesChanged = true
					break
				}
			}
		}
	}
	barShown := u.tabBarShown(numTabs)
	if barShown != ws.lastTabBarShown {
		ws.invalidateGridCache()
		ws.lastTabBarShown = barShown
	}
	ws.mu.Unlock()

	hoverTabIdx := ws.hoverTabIdx
	if ws.tabHoverLock {
		hoverTabIdx = -1
	}

	dragPreview := tabDragPreview{
		active: ws.dragActive,
		from:   ws.dragTabIdx,
		over:   ws.dragOverIdx,
		dx:     ws.dragDX,
	}

	if numTabs == 0 {
		return
	}

	bl := u.blockLayout(ws, fw, fh, numTabs)
	g := bl.G
	titleH := bl.TitleH

	termTop := bl.TermTop
	termH := bl.TermH
	termW := bl.TermW
	if termH <= 0 || termW <= 0 {
		return
	}
	newCols := termW / ws.cellW
	newRows := termH / ws.cellH
	if newCols < 1 {
		newCols = 1
	}
	if newRows < 1 {
		newRows = 1
	}
	// Center the terminal grid horizontally so left and right chrome gutters are equal.
	leftExtra := (termW - newCols*ws.cellW) / 2

	// Snapshot GPU and selection state.
	ws.mu.Lock()

	entry := ws.tabs[activeIdx]

	surfaceChanged := fw != ws.frameW || fh != ws.frameH
	gridChanged := newCols != ws.lastCols || newRows != ws.lastRows
	tabSwitch := activeIdx != ws.lastTabIdx || numTabs != ws.lastNumTabs
	tabScrollChanged := ws.tabScrollX != ws.lastTabScrollX
	hoverChanged := hoverTabIdx != ws.lastHoverTabIdx
	tabBarChanged := tabSwitch || titlesChanged || tabScrollChanged || hoverChanged

	prevCurRow := ws.lastCurRow
	prevCurCol := ws.lastCurCol

	var oldTex *gogpulib.Texture
	defer func() {
		if oldTex != nil {
			oldTex.Destroy()
		}
	}()
	bg := u.palette.bg

	var sel *selectionRange
	if entry.selActive {
		c0, r0, c1, r1 := entry.selNormalized()
		sel = &selectionRange{c0: c0, r0: r0, c1: c1, r1: r1}
	}
	selChanged := (sel != nil) != ws.lastSelActive
	layoutInProgress := ws.layoutInProgress
	inLiveResize := u.app.InSizeMove()

	if gridChanged {
		if ws.tex != nil && !inLiveResize {
			oldTex = ws.tex
			ws.tex = nil
		} else if inLiveResize {
			ws.pendingTexSync = true
		}
		needed := fw * fh * 4
		if needed > cap(ws.frame) {
			ws.frame = make([]byte, needed, needed+needed/4)
		} else {
			ws.frame = ws.frame[:needed]
		}
		if needed > cap(ws.frameAlt) {
			ws.frameAlt = make([]byte, needed, needed+needed/4)
		} else {
			ws.frameAlt = ws.frameAlt[:needed]
		}
		ws.frameW = fw
		ws.frameH = fh
		for i := 0; i < len(ws.frame); i += 4 {
			ws.frame[i] = bg.R
			ws.frame[i+1] = bg.G
			ws.frame[i+2] = bg.B
			ws.frame[i+3] = bg.A
		}
		ws.lastCols = newCols
		ws.lastRows = newRows
	} else if surfaceChanged {
		needed := fw * fh * 4
		if needed > cap(ws.frameAlt) {
			ws.frameAlt = make([]byte, needed, needed+needed/4)
		} else {
			ws.frameAlt = ws.frameAlt[:needed]
		}
		alt := ws.frameAlt
		for i := 0; i < len(alt); i += 4 {
			alt[i] = bg.R
			alt[i+1] = bg.G
			alt[i+2] = bg.B
			alt[i+3] = bg.A
		}
		srcStride := ws.frameW * 4
		dstStride := fw * 4
		copyW := ws.frameW
		if fw < copyW {
			copyW = fw
		}
		rowBytes := copyW * 4
		minH := ws.frameH
		if fh < minH {
			minH = fh
		}
		for row := 0; row < minH; row++ {
			copy(alt[row*dstStride:row*dstStride+rowBytes],
				ws.frame[row*srcStride:row*srcStride+rowBytes])
		}
		ws.frame, ws.frameAlt = alt, ws.frame
		ws.frameW = fw
		ws.frameH = fh
		// During live resize (Windows modal WM_ENTERSIZEMOVE / macOS drag),
		// recreating a full-window GPU texture every tick causes multi-GB growth
		// (same class of leak as Metal IOSurface churn). Keep the existing tex
		// and sync once when the drag ends.
		if !inLiveResize {
			if ws.tex != nil {
				oldTex = ws.tex
				ws.tex = nil
			}
		} else {
			ws.pendingTexSync = true
		}
	}

	// After live resize ends, force one texture recreate to match the final size.
	if !inLiveResize && ws.pendingTexSync {
		ws.pendingTexSync = false
		if ws.tex != nil && (ws.tex.Width() != fw || ws.tex.Height() != fh) {
			if oldTex == nil {
				oldTex = ws.tex
			} else {
				ws.tex.Destroy()
			}
			ws.tex = nil
		}
	}

	frame := ws.frame
	frameW := ws.frameW
	frameH := ws.frameH

	ws.mu.Unlock()

	// Spawn new shells at the exact onDraw grid before any resize pass.
	hadPendingPTY := ws.hasPendingPTY()
	if hadPendingPTY {
		u.spawnPendingPTYsAtGrid(ws, newCols, newRows)
	}

	// Resize terminals when the viewport grid changes.
	if gridChanged && !layoutInProgress && !hadPendingPTY {
		ws.mu.Lock()
		tabs := ws.tabs
		ws.mu.Unlock()
		if n == 1 {
			logger.Get().Debug("first onDraw grid", "cols", newCols, "rows", newRows, "fw", fw, "fh", fh, "cellW", ws.cellW, "cellH", ws.cellH)
		}
		resizeTabsToGrid(tabs, newCols, newRows)
	} else if tabSwitch && !layoutInProgress && !hadPendingPTY && (entry.term.Cols() != newCols || entry.term.Rows() != newRows) {
		resizeTabsToGrid([]*tabSlot{entry}, newCols, newRows)
	}

	scrollOff := int(entry.scrollOffset.Load())
	scrollChanged := scrollOff != ws.lastScrollOffset
	curCol, curRow := entry.term.CursorPos()
	termScrollGen := entry.term.ScrollGen()
	termScrolled := termScrollGen != ws.lastScrollGen

	blinkOn := true
	if u.blinkEnabled {
		blinkOn = u.blinkOn.Load()
	}
	// After leaving scrollback, force the cursor visible for this frame so blink
	// phase off cannot leave a blank cell until the next toggle.
	if scrollChanged && scrollOff == 0 {
		blinkOn = true
		u.blinkOn.Store(true)
	}
	blinkChanged := blinkOn != ws.lastBlinkOn
	// Show block cursor whenever we're at the live viewport (ignore brief DECTCEM off).
	showCursor := scrollOff == 0 && blinkOn

	// Soft-wrap at the bottom scrolls the grid: must repaint every row or the
	// prompt / start of the input line stays as stale pixels (Kitty keeps them).
	forceAll := gridChanged || scrollChanged || termScrolled || selChanged || blinkChanged || (tabSwitch && !dragPreview.active)

	// Paint native title underlay (macOS FullSizeContent) + tab bar.
	tabPainted := false
	winTitle := ""
	if titleH > 0 {
		if tp := entry.liveTitle.Load(); tp != nil && *tp != "" {
			winTitle = *tp
		} else if u.cfg.Window.Title != "" {
			winTitle = u.cfg.Window.Title
		} else {
			winTitle = defaultTitle
		}
	}
	if titleH > 0 || tbH > 0 {
		needChrome := tabBarChanged || dragPreview.active || forceAll ||
			titleH > 0 && (gridChanged || surfaceChanged || ws.tex == nil)
		if needChrome {
			if titleH > 0 {
				paintTitleChrome(frame, frameW, titleH, &u.palette, ws.face, winTitle, titleAlignForPaint(u.cfg), ws.cellAscent, tbH > 0)
			}
			if tbH > 0 && bl.TabTop >= 0 {
				renderTabBar(frame, frameW, frameH, tbH, ws.cellW, ws.cellH, ws.cellAscent, g,
					ws.face, &u.palette, tabInfos, activeIdx, hoverTabIdx, dragPreview, ws.scaleFactor, u.cfg.Tabs.Size, ws.tabScrollX, bl.TabTop)
				ws.lastTabTitles = make([]string, len(tabInfos))
				for i, ti := range tabInfos {
					ws.lastTabTitles[i] = ti.title
				}
			}
			tabPainted = true
		}
	}

	// Thin chrome hairline between tab bar and terminal.
	if tbH > 0 && bl.TabTop >= 0 && bl.TabGap > 0 && (tabPainted || forceAll || gridChanged || surfaceChanged) {
		y0 := bl.TabTop + tbH
		paintInterBlockGap(frame, frameW, frameH, y0, bl.TermTop)
	}

	contentL := g + leftExtra
	contentT := termTop
	contentR := contentL + newCols*ws.cellW
	contentB := termTop + newRows*ws.cellH
	chromeTop := bl.ChromeTop
	cornerR := contentCornerRadius(g)
	if forceAll || gridChanged || surfaceChanged || ws.tex == nil {
		applyEdgeChrome(frame, frameW, frameH, contentL, contentT, contentR, contentB, chromeTop, cornerR, &u.palette)
	}

	rctx := renderCtx{
		pal:         &u.palette,
		cursorShape: u.cursorShape,
		showCursor:  showCursor,
		scrollOff:   scrollOff,
		prevCurRow:  prevCurRow,
		forceAll:    forceAll,
		sel:         sel,
		clipL:       contentL,
		clipT:       contentT,
		clipR:       contentR,
		clipB:       contentB,
		clipRad:     cornerR,
		clipSet:     cornerR > 0,
	}

	// Render the terminal into the padded viewport below the tab bar.
	// paintTerminalBackdrop is only needed on full repaints: it fills the
	// entire inset (including fractional pixels beyond the last grid row/col)
	// with pal.bg. On cursor-only frames the buffer retains the previous frame,
	// so calling it would blank all non-dirty rows before render repaints them,
	// producing a one-frame flicker on every cursor move.
	entry.parseMu.Lock()
	if forceAll || gridChanged || surfaceChanged || ws.tex == nil {
		paintTerminalBackdrop(frame, frameW, frameH, contentL, contentT, contentR, contentB, cornerR, &u.palette)
	}
	painted := render(entry.term, rctx, frame, frameW, frameH, ws.cellW, ws.cellH, ws.cellAscent, termTop, contentL, ws.face, ws.faceFallback)
	if painted && scrollOff == 0 {
		entry.term.ClearDirty()
		entry.dirty.Store(false)
		ws.dirty.Store(false)
	}
	entry.parseMu.Unlock()

	ws.lastScrollOffset = scrollOff
	ws.lastScrollGen = termScrollGen
	ws.lastSelActive = sel != nil
	ws.lastBlinkOn = blinkOn
	ws.lastTabIdx = activeIdx
	ws.lastNumTabs = numTabs
	ws.lastTabScrollX = ws.tabScrollX
	ws.lastHoverTabIdx = hoverTabIdx

	if gridChanged {
		ws.pendingGC = true
	}
	if ws.pendingGC && !u.app.InSizeMove() {
		ws.pendingGC = false
		runtime.GC()
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	if frameW <= 0 || frameH <= 0 {
		return
	}

	needUpdate := painted || tabPainted || curRow != prevCurRow || curCol != prevCurCol ||
		scrollChanged || selChanged || blinkChanged || gridChanged || surfaceChanged || ws.tex == nil
	if needUpdate {
		applyEdgeChromeAfterGlyphs(frame, frameW, frameH, contentL, contentT, contentR, contentB, chromeTop, cornerR, &u.palette)
	}

	// During live resize we may keep a previous-size GPU texture to avoid
	// per-tick allocation storms. Present it as-is until the drag ends.
	if ws.tex != nil && shouldDeferGPUTexRecreate(inLiveResize, ws.tex.Width(), ws.tex.Height(), frameW, frameH) {
		chrome := chromePanel(&u.palette)
		ctx.Clear(float32(chrome.R)/255, float32(chrome.G)/255, float32(chrome.B)/255, 1)
		if err := ctx.PresentTexture(ws.tex); err != nil {
			u.debugf("onDraw #%d present(stale) err: %v", n, err)
		}
		return
	}
	if ws.tex != nil && (ws.tex.Width() != frameW || ws.tex.Height() != frameH) {
		if oldTex == nil {
			oldTex = ws.tex
		} else {
			ws.tex.Destroy()
		}
		ws.tex = nil
	}

	if ws.tex == nil {
		tex, err := ctx.Renderer().NewTextureFromRGBA(frameW, frameH, frame)
		if err != nil {
			ws.texErrCount++
			u.debugf("onDraw #%d new tex err (%d): %v", n, ws.texErrCount, err)
			if ws.texErrCount == 1 || ws.texErrCount == 10 {
				logger.Get().Warn("gogpu: texture allocation failed",
					"consecutive", ws.texErrCount, "err", err)
			} else if ws.texErrCount > 10 {
				logger.Get().Error("gogpu: texture allocation failed repeatedly, giving up",
					"consecutive", ws.texErrCount)
			}
			if ws.texErrCount <= 10 {
				u.app.RequestRedraw()
			}
			return
		}
		ws.texErrCount = 0
		ws.tex = tex
	} else if needUpdate {
		if err := ws.tex.UpdateData(frame); err != nil {
			u.debugf("onDraw #%d update err: %v", n, err)
			logger.Get().Warn("gogpu: texture update failed", "err", err)
			u.app.RequestRedraw()
			return
		}
	}

	ws.lastCurRow = curRow
	ws.lastCurCol = curCol

	chrome := chromePanel(&u.palette)
	br := float32(chrome.R) / 255
	bgc := float32(chrome.G) / 255
	bb := float32(chrome.B) / 255
	ctx.Clear(br, bgc, bb, 1)
	if err := ctx.PresentTexture(ws.tex); err != nil {
		u.debugf("onDraw #%d present err: %v", n, err)
		logger.Get().Warn("gogpu: present texture failed", "err", err)
		u.app.RequestRedraw()
		return
	}

	u.debugf("onDraw #%d ok tex=%dx%d grid=%dx%d painted=%v tabPainted=%v dirty=%v",
		n, frameW, frameH, ws.lastCols, ws.lastRows, painted, tabPainted, ws.dirty.Load())
}

// ── Scroll handling ───────────────────────────────────────────────────────────

func (u *UI) handleScroll(ws *winState, ev gpucontext.ScrollEvent) {
	if ev.DeltaY == 0 && ev.DeltaX == 0 {
		return
	}

	physY := ws.phys(ev.Y)
	ws.mu.Lock()
	numTabs := len(ws.tabs)
	ws.mu.Unlock()
	tbH := u.tabBarHeight(ws, numTabs)

	// Horizontal scroll over the tab bar (Rio / Wails overflow).
	if tbH > 0 && ws.hasFrameSize() {
		fw, fh := ws.physicalFrameSize()
		bl := u.blockLayout(ws, fw, fh, numTabs)
		if bl.TabTop >= 0 && physY >= bl.TabTop && physY < bl.TabTop+tbH {
			tabW, _, tabsAreaW := computeTabLayout(fw, bl.G, numTabs, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size)
			maxScroll := tabScrollMax(numTabs, tabW, tabsAreaW)
			if maxScroll <= 0 {
				return
			}
			// Trackpads send DeltaX; mice often send vertical wheel — treat both as strip scroll.
			delta := int(math.Round(ev.DeltaX))
			if delta == 0 {
				delta = int(math.Round(ev.DeltaY))
			}
			if sf := ws.scaleFactor; sf > 0 {
				delta = int(float64(delta) * sf)
			}
			// Amplify a bit so a one-notch wheel moves ~half a compact tab.
			if delta != 0 && absInt(delta) < tabW/2 {
				if delta > 0 {
					delta = tabW / 2
				} else {
					delta = -(tabW / 2)
				}
			}
			ws.tabScrollX = clampTabScroll(ws.tabScrollX+delta, maxScroll)
			u.app.RequestRedraw()
			return
		}
	}

	if ev.DeltaY == 0 {
		return
	}

	ws.mu.Lock()
	entry := ws.activeTab()
	ws.mu.Unlock()
	if entry == nil {
		return
	}

	sbLen := entry.term.ScrollbackLen()
	if sbLen == 0 {
		return
	}

	current := int(entry.scrollOffset.Load())
	var delta int
	switch ev.DeltaMode {
	case gpucontext.ScrollDeltaLine:
		delta = int(math.Round(ev.DeltaY))
	case gpucontext.ScrollDeltaPage:
		delta = int(math.Round(ev.DeltaY)) * entry.term.Rows()
	default:
		sf := ws.scaleFactor
		if sf <= 0 {
			sf = 1
		}
		logicalCellH := float64(ws.cellH) / sf
		if logicalCellH > 0 {
			delta = int(math.Round(ev.DeltaY / logicalCellH))
		} else {
			delta = int(math.Round(ev.DeltaY / 16))
		}
	}

	next := current - delta
	if next < 0 {
		next = 0
	}
	if next > sbLen {
		next = sbLen
	}
	if next == current {
		return
	}
	entry.scrollOffset.Store(int64(next))

	ws.mu.Lock()
	entry.selActive = false
	ws.mu.Unlock()

	u.app.RequestRedraw()
}

// ── Key handling ──────────────────────────────────────────────────────────────

func (u *UI) handleKeyPress(ws *winState, key gpucontext.Key, mods gpucontext.Modifiers) {
	ws.mu.Lock()
	entry := ws.activeTab()
	ws.mu.Unlock()
	if entry == nil {
		return
	}

	u.debugf("key key=%v mods=%v", key, mods)

	// Modifier-only keys (Cmd/Ctrl/Shift/Alt) must NOT clear selection —
	// otherwise pressing Cmd alone clears it before Cmd+C arrives.
	if isModifierKey(key) {
		return
	}

	// CapsLock and NumLock are always stripped for shortcut matching.
	// On Windows, NumLock is typically on and would otherwise break every
	// Ctrl+Shift+? comparison. This mirrors matchBinding's behavior.
	cleanMods := mods &^ (gpucontext.ModCapsLock | gpucontext.ModNumLock)

	// Platform tab/window shortcuts before clearing keyHandled — macOS may deliver
	// TextInput for the letter before KeyPress returns.
	if isMac {
		if cleanMods == gpucontext.ModSuper {
			switch key {
			case gpucontext.KeyT:
				if u.cfg.Tabs.Enabled {
					u.blockShortcutTextInput(ws)
					u.addNewTab(ws)
				}
				return
			case gpucontext.KeyN:
				u.openNewWindow()
				ws.keyHandled = true
				return
			case gpucontext.KeyW:
				u.closeCurrentTab(ws)
				ws.keyHandled = true
				return
			}
		}
		if cleanMods == gpucontext.ModSuper|gpucontext.ModShift {
			switch key {
			case gpucontext.KeyRightBracket:
				u.switchTab(ws, +1)
				ws.keyHandled = true
				return
			case gpucontext.KeyLeftBracket:
				u.switchTab(ws, -1)
				ws.keyHandled = true
				return
			}
		}
	} else {
		if cleanMods == gpucontext.ModControl|gpucontext.ModShift {
			switch key {
			case gpucontext.KeyT:
				if u.cfg.Tabs.Enabled {
					u.blockShortcutTextInput(ws)
					u.addNewTab(ws)
				}
				return
			case gpucontext.KeyN:
				u.openNewWindow()
				ws.keyHandled = true
				return
			case gpucontext.KeyW:
				u.closeCurrentTab(ws)
				ws.keyHandled = true
				return
			}
		}
		if cleanMods == gpucontext.ModControl && !mods.HasAlt() && !mods.HasSuper() {
			if key == gpucontext.KeyTab {
				if mods.HasShift() {
					u.switchTab(ws, -1)
				} else {
					u.switchTab(ws, +1)
				}
				ws.keyHandled = true
				return
			}
		}
	}

	// Fresh keypress — clear stale suppress flag from a prior special key with
	// no following TextInput (e.g. Backspace alone).
	ws.keyHandled = false

	// Platform shortcuts (mirror frontend/src/keybindings.ts).
	// Copy/Paste/Scroll/Font must NOT clear the current selection.
	if action := matchPlatformBinding(key, mods, isMac); action != "" {
		u.debugf("key platform action=%s", action)
		u.handleAction(ws, action)
		ws.keyHandled = true
		return
	}

	// Config keybindings.
	if action := matchBinding(key, mods, u.bindings); action != "" {
		u.debugf("key config action=%s", action)
		u.handleAction(ws, action)
		ws.keyHandled = true
		return
	}

	seq := keyToSeq(key, mods, entry.term.AppCursorKeys())
	if len(seq) == 0 {
		return
	}
	// Real typing clears selection (same as xterm when input is sent).
	ws.mu.Lock()
	entry.selActive = false
	ws.mu.Unlock()
	entry.scrollOffset.Store(0)
	// Only suppress following TextInput for keys that also emit TextInput
	// (Enter/Tab). Backspace/arrows must NOT leave keyHandled stuck — that ate
	// the next printable character (needed two keypresses after delete).
	switch key {
	case gpucontext.KeyEnter, gpucontext.KeyTab:
		ws.keyHandled = true
	default:
		ws.keyHandled = false
	}
	entry.sendInput(seq)
}

func isModifierKey(key gpucontext.Key) bool {
	switch key {
	case gpucontext.KeyLeftShift, gpucontext.KeyRightShift,
		gpucontext.KeyLeftControl, gpucontext.KeyRightControl,
		gpucontext.KeyLeftAlt, gpucontext.KeyRightAlt,
		gpucontext.KeyLeftSuper, gpucontext.KeyRightSuper,
		gpucontext.KeyCapsLock, gpucontext.KeyNumLock:
		return true
	}
	return false
}

// switchTab advances the active tab by delta (+1 = next, -1 = prev) with wrap-around.
func (u *UI) switchTab(ws *winState, delta int) {
	ws.mu.Lock()
	n := len(ws.tabs)
	if n > 0 {
		ws.activeTabIdx = ((ws.activeTabIdx+delta)%n + n) % n
	}
	idx := ws.activeTabIdx
	var title string
	if idx >= 0 && idx < len(ws.tabs) {
		if tp := ws.tabs[idx].liveTitle.Load(); tp != nil {
			title = *tp
		}
	}
	ws.mu.Unlock()
	u.syncWindowTitle(ws, title)
	u.app.RequestRedraw()
}

// syncWindowTitle sets the OS window title from the active tab's OSC title.
// On macOS FullSizeContent we paint the title ourselves centered — keep the native
// NSTextField empty so it does not fight with the GPU chrome.
func (u *UI) syncWindowTitle(ws *winState, title string) {
	if title == "" {
		title = u.cfg.Window.Title
		if title == "" {
			title = defaultTitle
		}
	}
	paintTitle := title
	nativeTitle := title
	if isMac && u.cfg.Window.ShowTitleBar {
		// HeaderAlignRight injects an NSTextField; blank it so only our paint shows.
		nativeTitle = " "
	}
	_ = paintTitle
	if ws == u.primary || (ws.win != nil && ws.win.ID() == u.primaryID) {
		u.app.SetTitle(nativeTitle)
	} else if ws.win != nil {
		ws.win.SetTitle(nativeTitle)
	}
}

// handleAction executes a named keybinding action.
func (u *UI) handleAction(ws *winState, action string) {
	ws.mu.Lock()
	entry := ws.activeTab()
	ws.mu.Unlock()

	switch action {
	case actionPaste:
		u.pasteToTab(entry)

	case actionCopy:
		text := ws.extractSelectedText()
		if text == "" {
			u.debugf("copy: empty selection (selActive?)")
			return
		}
		u.debugf("copy: selection %q (%d bytes)", text, len(text))
		if err := u.clipboardWrite(text); err != nil {
			u.debugf("copy: write failed: %v", err)
			return
		}
		u.debugf("copy: ok")

	case "Clear":
		if entry == nil {
			return
		}
		entry.scrollOffset.Store(0)
		// Match Wails/xterm: erase scrollback locally, then form-feed to the shell.
		entry.parseMu.Lock()
		entry.parser.Advance(entry.term, []byte("\x1b[3J"))
		entry.parseMu.Unlock()
		entry.sendInput([]byte{0x0c})
		if fn := entry.pokePTY; fn != nil {
			fn()
		} else {
			refreshPTYSize(entry)
		}
		ws.dirty.Store(true)
		u.app.RequestRedraw()

	case "ScrollUp":
		u.scrollBy(ws, 1)
	case "ScrollDown":
		u.scrollBy(ws, -1)
	case "ScrollPageUp":
		if entry != nil {
			u.scrollBy(ws, entry.term.Rows()-1)
		}
	case "ScrollPageDown":
		if entry != nil {
			u.scrollBy(ws, -(entry.term.Rows() - 1))
		}
	case "ScrollTop":
		if entry != nil {
			entry.scrollOffset.Store(int64(entry.term.ScrollbackLen()))
			u.app.RequestRedraw()
		}
	case "ScrollBottom":
		if entry != nil {
			entry.scrollOffset.Store(0)
			u.app.RequestRedraw()
		}

	case "FontIncrease":
		next := u.fontSizeDesired + 1
		if next > 32 {
			next = 32
		}
		u.fontSizeDesired = next
		u.app.RequestRedraw()
	case "FontDecrease":
		next := u.fontSizeDesired - 1
		if next < 6 {
			next = 6
		}
		u.fontSizeDesired = next
		u.app.RequestRedraw()
	case "FontReset":
		u.fontSizeDesired = u.baseFontSize
		u.app.RequestRedraw()

	case "NewWindow":
		u.openNewWindow()
	case "NewTab":
		if u.cfg.Tabs.Enabled {
			u.addNewTab(ws)
		}
	}
}

// pasteToTab reads the OS clipboard and writes it to the active tab's PTY.
func (u *UI) pasteToTab(entry *tabSlot) {
	if entry == nil {
		return
	}
	text, err := u.clipboardRead()
	if err != nil {
		u.debugf("paste: clipboard read error: %v", err)
		return
	}
	if text == "" {
		u.debugf("paste: clipboard empty")
		return
	}
	u.pasteTextToTab(entry, text)
}

// pasteTextToTab writes clipboard text to the PTY with bracketed-paste wrapping.
func (u *UI) pasteTextToTab(entry *tabSlot, text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return
	}
	if entry.term.BracketedPaste() {
		text = "\x1b[200~" + text + "\x1b[201~"
	}
	entry.sendInput([]byte(text))
}

// ── Pointer handling ──────────────────────────────────────────────────────────

// phys converts a logical-pixel pointer coordinate to physical pixels.
func (ws *winState) phys(v float64) int {
	sf := ws.scaleFactor
	if sf <= 0 {
		sf = 1
	}
	return int(v * sf)
}

const tabDragThreshold = 6 // logical pixels, same as frontend TabBar.tsx

func (u *UI) handlePointer(ws *winState, ev gpucontext.PointerEvent) {
	// All hit-testing is done in physical pixels; pointer events arrive in logical pixels.
	physX := ws.phys(ev.X)
	physY := ws.phys(ev.Y)

	// Right-click and middle-click = paste (Wails context menu + Linux primary selection).
	if (ev.Button == gpucontext.ButtonRight || ev.Button == gpucontext.ButtonMiddle) &&
		ev.Type == gpucontext.PointerDown {
		ws.mu.Lock()
		entry := ws.activeTab()
		ws.mu.Unlock()
		u.pasteToTab(entry)
		return
	}

	// Continue tab press/drag even when the pointer leaves the tab bar.
	if ws.tabPressActive || ws.dragActive {
		if u.handleTabPointer(ws, ev, physX, physY) {
			return
		}
	}

	if ev.Button != gpucontext.ButtonLeft && ev.Type != gpucontext.PointerMove {
		return
	}
	if ev.Type != gpucontext.PointerMove && ev.Button != gpucontext.ButtonLeft {
		return
	}

	// Determine tab bar height.
	ws.mu.Lock()
	numTabs := len(ws.tabs)
	ws.mu.Unlock()
	tbH := u.tabBarHeight(ws, numTabs)
	if !ws.hasFrameSize() {
		return
	}
	fw, fh := ws.physicalFrameSize()
	bl := u.blockLayout(ws, fw, fh, numTabs)

	// Hover-tab tracking: update hoverTabIdx on every pointer move.
	if ev.Type == gpucontext.PointerMove && tbH > 0 && bl.TabTop >= 0 {
		if ws.tabHoverLock {
			ws.tabHoverLock = false
		}
		if physY >= bl.TabTop && physY < bl.TabTop+tbH {
			padX := u.termPadding(ws)
			ws.mu.Lock()
			activeForHover := ws.activeTabIdx
			ws.mu.Unlock()
			newHover := tabIndexAtX(physX, fw, padX, numTabs, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size, ws.tabScrollX, activeForHover)
			if newHover != ws.hoverTabIdx {
				ws.hoverTabIdx = newHover
				u.app.RequestRedraw()
			}
		} else if ws.hoverTabIdx != -1 {
			ws.hoverTabIdx = -1
			u.app.RequestRedraw()
		}
	}

	// Tab bar area (below title gutter).
	if tbH > 0 && bl.TabTop >= 0 && physY >= bl.TabTop && physY < bl.TabTop+tbH {
		u.handleTabBarPointer(ws, ev, physX, physY, numTabs)
		return
	}

	// Terminal area — selection (account for uniform gutter).
	cW, cH := ws.cellW, ws.cellH
	if cW <= 0 || cH <= 0 {
		return
	}
	termTop := bl.TermTop
	adjX := physX - bl.G
	adjY := physY - termTop
	if adjX < 0 || adjY < 0 {
		return
	}

	ws.mu.Lock()
	entry := ws.activeTab()
	if entry == nil {
		ws.mu.Unlock()
		return
	}

	col := clampInt(adjX/cW, entry.term.Cols()-1)
	row := clampInt(adjY/cH, entry.term.Rows()-1)

	switch ev.Type {
	case gpucontext.PointerDown:
		now := time.Now()
		doubleClick := ws.clickCount > 0 &&
			ws.lastClickCol == col && ws.lastClickRow == row &&
			now.Sub(ws.lastClickTime) < 300*time.Millisecond

		if doubleClick {
			ws.clickCount = 0
			ws.mouseDown = false
			ws.mu.Unlock()
			startCol, endCol := computeWordBounds(entry.term, col, row)
			ws.mu.Lock()
			entry.selActive = true
			entry.selStartCol, entry.selEndCol = startCol, endCol
			entry.selStartRow, entry.selEndRow = row, row
		} else {
			ws.clickCount = 1
			ws.lastClickTime = now
			ws.lastClickCol = col
			ws.lastClickRow = row
			ws.mouseDown = true
			entry.selActive = true
			entry.selStartCol = col
			entry.selStartRow = row
			entry.selEndCol = col
			entry.selEndRow = row
		}

	case gpucontext.PointerMove:
		if ws.mouseDown {
			entry.selEndCol = col
			entry.selEndRow = row
		}

	case gpucontext.PointerUp:
		ws.mouseDown = false
		// Single click with no drag = no selection; drag = keep selection.
		if entry.selStartCol == entry.selEndCol && entry.selStartRow == entry.selEndRow {
			entry.selActive = false
		}
	}

	ws.mu.Unlock()
	u.app.RequestRedraw()
}

// handleTabBarPointer processes left-button events in the tab bar area (physical pixels).
func (u *UI) handleTabBarPointer(ws *winState, ev gpucontext.PointerEvent, physX, physY, numTabs int) {
	fw, _ := ws.physicalFrameSize()
	padX := u.termPadding(ws)
	tabW, plusX, tabsAreaW := computeTabLayout(fw, padX, numTabs, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size)
	plusW := plusBtnWidth(ws.cellW, ws.scaleFactor)
	maxScroll := tabScrollMax(numTabs, tabW, tabsAreaW)
	ws.tabScrollX = clampTabScroll(ws.tabScrollX, maxScroll)
	origin := padX
	if origin < 0 {
		origin = 0
	}

	switch ev.Type {
	case gpucontext.PointerDown:
		if tabW > 0 && physX >= plusX && physX < plusX+plusW {
			if u.cfg.Tabs.Enabled {
				u.addNewTab(ws)
			}
			return
		}
		if tabW <= 0 || numTabs == 0 || physX < origin || physX >= origin+tabsAreaW {
			return
		}

		ws.mu.Lock()
		activeForHit := ws.activeTabIdx
		ws.mu.Unlock()
		tabIdx := tabIndexAtX(physX, fw, padX, numTabs, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size, ws.tabScrollX, activeForHit)
		if tabIdx < 0 {
			return
		}

		// Close button: use visual X (sticky pin when overflow).
		tabLeft := origin + tabIdx*tabW - ws.tabScrollX
		if pinL, pinR := activeTabPin(activeForHit, numTabs, tabW, tabsAreaW, ws.tabScrollX); tabIdx == activeForHit {
			if pinL {
				tabLeft = origin
			} else if pinR {
				tabLeft = origin + tabsAreaW - tabW
				if tabLeft < origin {
					tabLeft = origin
				}
			}
		}
		if numTabs > 1 {
			closeX := tabLeft + tabW - ws.cellW - 12
			if physX >= closeX && physX < tabLeft+tabW {
				u.closeTabAtIndex(ws, tabIdx)
				return
			}
		}

		ws.tabPressActive = true
		ws.tabPressIdx = tabIdx
		ws.tabPressX = physX
		ws.tabPressY = physY
		ws.dragOverIdx = tabIdx

	case gpucontext.PointerMove, gpucontext.PointerUp, gpucontext.PointerCancel:
		u.handleTabPointer(ws, ev, physX, physY)
	}
}

// handleTabPointer continues an in-progress tab press or drag. Returns true when consumed.
func (u *UI) handleTabPointer(ws *winState, ev gpucontext.PointerEvent, physX, physY int) bool {
	if !ws.tabPressActive && !ws.dragActive {
		return false
	}

	ws.mu.Lock()
	numTabs := len(ws.tabs)
	ws.mu.Unlock()
	if numTabs == 0 {
		ws.resetTabPointer()
		return true
	}

	switch ev.Type {
	case gpucontext.PointerMove:
		if ws.tabPressActive && !ws.dragActive {
			thresh := tabDragThreshold
			if sf := ws.scaleFactor; sf > 0 {
				thresh = int(float64(tabDragThreshold) * sf)
			}
			dx := physX - ws.tabPressX
			dy := physY - ws.tabPressY
			if dx*dx+dy*dy < thresh*thresh {
				return true
			}
			ws.dragActive = true
			ws.dragTabIdx = ws.tabPressIdx
			ws.dragOverIdx = ws.tabPressIdx
		}
		if ws.dragActive && ws.hasFrameSize() {
			fw, _ := ws.physicalFrameSize()
			padX := u.termPadding(ws)
			tabW, _, tabsAreaW := computeTabLayout(fw, padX, numTabs, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size)
			maxScroll := tabScrollMax(numTabs, tabW, tabsAreaW)
			ws.tabScrollX = clampTabScroll(ws.tabScrollX, maxScroll)
			origin := padX
			if origin < 0 {
				origin = 0
			}

			// Edge auto-scroll while dragging (Rio / Wails).
			edge := 40
			if sf := ws.scaleFactor; sf > 0 {
				edge = int(40 * sf)
			}
			localX := physX - origin
			if localX < edge {
				ws.tabScrollX = clampTabScroll(ws.tabScrollX-8, maxScroll)
			} else if localX > tabsAreaW-edge {
				ws.tabScrollX = clampTabScroll(ws.tabScrollX+8, maxScroll)
			}

			// Drop target from midpoint of scrolled slots (not raw index under cursor).
			contentX := localX + ws.tabScrollX
			over := 0
			for i := 0; i < numTabs; i++ {
				mid := i*tabW + tabW/2
				if contentX < mid {
					over = i
					break
				}
				over = i
			}
			ws.dragOverIdx = over

			// Keep preview inside the strip: dragDX is offset from press,
			// but visual x is clamped in renderTabBar.
			ws.dragDX = physX - ws.tabPressX
			u.app.RequestRedraw()
		}
		return true

	case gpucontext.PointerUp, gpucontext.PointerCancel:
		if ws.dragActive {
			from, over := ws.dragTabIdx, ws.dragOverIdx
			// Snap: commit reorder only when midpoint crossed a different slot;
			// otherwise the tab springs back (dragDX cleared in reset).
			if from != over && over >= 0 {
				u.reorderTab(ws, from, over)
				ws.mu.Lock()
				if over < len(ws.tabs) {
					ws.activeTabIdx = over
				}
				title := ""
				if tp := ws.tabs[ws.activeTabIdx].liveTitle.Load(); tp != nil {
					title = *tp
				}
				ws.mu.Unlock()
				u.syncWindowTitle(ws, title)
				u.ensureTabVisible(ws, ws.activeTabIdx)
			} else {
				ws.mu.Lock()
				ws.activeTabIdx = from
				title := ""
				if from >= 0 && from < len(ws.tabs) {
					if tp := ws.tabs[from].liveTitle.Load(); tp != nil {
						title = *tp
					}
				}
				ws.mu.Unlock()
				u.syncWindowTitle(ws, title)
			}
		} else if ws.tabPressActive {
			ws.mu.Lock()
			ws.activeTabIdx = ws.tabPressIdx
			title := ""
			if ws.tabPressIdx >= 0 && ws.tabPressIdx < len(ws.tabs) {
				if tp := ws.tabs[ws.tabPressIdx].liveTitle.Load(); tp != nil {
					title = *tp
				}
			}
			ws.mu.Unlock()
			u.syncWindowTitle(ws, title)
			u.ensureTabVisible(ws, ws.tabPressIdx)
		}
		ws.resetTabPointer()
		u.app.RequestRedraw()
		return true
	}
	return true
}

// ensureTabVisible scrolls the tab strip so idx is inside the visible area.
func (u *UI) ensureTabVisible(ws *winState, idx int) {
	if idx < 0 || !ws.hasFrameSize() {
		return
	}
	ws.mu.Lock()
	numTabs := len(ws.tabs)
	ws.mu.Unlock()
	fw, _ := ws.physicalFrameSize()
	padX := u.termPadding(ws)
	tabW, _, tabsAreaW := computeTabLayout(fw, padX, numTabs, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size)
	maxScroll := tabScrollMax(numTabs, tabW, tabsAreaW)
	if maxScroll <= 0 || tabW <= 0 {
		ws.tabScrollX = 0
		return
	}
	left := idx * tabW
	right := left + tabW
	if left < ws.tabScrollX {
		ws.tabScrollX = left
	} else if right > ws.tabScrollX+tabsAreaW {
		ws.tabScrollX = right - tabsAreaW
	}
	ws.tabScrollX = clampTabScroll(ws.tabScrollX, maxScroll)
}

// reorderTab moves a tab from one index to another and fixes the active index.
func (u *UI) reorderTab(ws *winState, from, to int) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	n := len(ws.tabs)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return
	}

	active := ws.activeTabIdx
	ws.tabs = reorderTabSlots(ws.tabs, from, to)

	switch {
	case active == from:
		ws.activeTabIdx = to
	case from < active && to >= active:
		ws.activeTabIdx--
	case from > active && to <= active:
		ws.activeTabIdx++
	}
}

// shellBaseName returns the executable basename without a .exe suffix.
func shellBaseName(path string) string {
	base := filepath.Base(strings.ReplaceAll(path, `\`, `/`))
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".EXE")
	return base
}

// shellDisplayName returns a short, user-facing label for a shell path
// (e.g. powershell.exe → "PowerShell"). Used as a last-resort title only.
func shellDisplayName(path string) string {
	const shellZsh = "zsh"
	switch strings.ToLower(shellBaseName(path)) {
	case "powershell", "pwsh":
		return "PowerShell"
	case "cmd":
		return "Command Prompt"
	case "bash":
		return "bash"
	case shellZsh:
		return shellZsh
	case "fish":
		return "fish"
	default:
		return shellBaseName(path)
	}
}

// startupCwdTitle returns the directory the shell starts in (home), so tabs
// match `PS C:\Users\...>` before the first OSC update.
func startupCwdTitle() string {
	if runtime.GOOS == osWindows {
		if p := os.Getenv("USERPROFILE"); p != "" {
			return p
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return ""
}

// normalizeOSCTitle cleans titles from the shell / ConPTY.
// Windows conhost often sets OSC 0/2 to the process image path
// (C:\...\powershell.exe); ignore those so a cwd title is not overwritten.
func normalizeOSCTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(title, `\`, `/`))
	if strings.HasSuffix(strings.ToLower(base), ".exe") {
		return ""
	}
	return title
}

func clampInt(v, hi int) int {
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// resizeTabsToGrid updates every tab's terminal grid and notifies PTYs.
func resizeTabsToGrid(tabs []*tabSlot, cols, rows int) {
	ptyCols, ptyRows := pty.ClampSize(cols, rows)
	for _, tab := range tabs {
		if tab == nil || tab.needsPTY {
			continue
		}
		if tab.term.Cols() != cols || tab.term.Rows() != rows {
			tab.term.Resize(cols, rows)
		}
		if tab.ptyW == ptyCols && tab.ptyH == ptyRows {
			continue
		}
		if fn := tab.resizeFn; fn != nil {
			fn(adapter.ResizeEvent{Cols: ptyCols, Rows: ptyRows})
		}
	}
}

// ── Scroll helper ─────────────────────────────────────────────────────────────

func (u *UI) scrollBy(ws *winState, lines int) {
	if lines == 0 {
		return
	}
	ws.mu.Lock()
	entry := ws.activeTab()
	ws.mu.Unlock()
	if entry == nil {
		return
	}
	sbLen := entry.term.ScrollbackLen()
	current := int(entry.scrollOffset.Load())
	next := current + lines
	if next < 0 {
		next = 0
	}
	if next > sbLen {
		next = sbLen
	}
	if next == current {
		return
	}
	entry.scrollOffset.Store(int64(next))
	u.app.RequestRedraw()
}
