// Package gogpu provides the GPU-accelerated windowing backend.
// gogpu manages the OS window + input; gg+ggcanvas render terminal cells to GPU.
package gogpu

import (
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/util/config"
)

// WindowFactory is called to create a new terminal session for an additional window.
// cols and rows are the initial PTY dimensions derived from the configured window
// size and font metrics, so the PTY starts at the correct size without a resize event.
type WindowFactory func(cols, rows uint16) (term *terminal.Terminal, write func([]byte), resize func(uint16, uint16), cleanup func())

// Gogpu is the windowing backend: gogpu manages the window and input,
// gg+ggcanvas render terminal cells directly to the GPU surface.
// Implements adapter.UI; also exposes RequestRedraw for the PTY goroutine.
type Gogpu struct {
	cfg  *config.Config
	term *terminal.Terminal // primary window's terminal

	app    *gogpu.App
	canvas *ggcanvas.Canvas // primary window's canvas

	// Shared font state (all windows use the same font).
	fontSrc  *text.FontSource
	fontFace text.Face
	cellW    float64 // logical cell width (pixels)
	cellH    float64 // logical cell height (pixels)
	ascent   float64 // baseline offset from top of cell
	padX     float64 // horizontal padding (pixels)
	padY     float64 // vertical padding (pixels)

	descent float64 // baseline-to-bottom distance (without LineGap)

	pendingTitle atomic.Pointer[string] // set from PTY goroutine, applied in onDraw (main thread)

	initDone atomic.Bool // true after first successful onDraw init

	// WindowFactory is set by app.go before Run(). Called on Cmd+N.
	WindowFactory WindowFactory

	// Text selection (primary window).
	sel windowSelection

	// Secondary window sessions.
	mu      sync.Mutex
	windows []*termWindow

	keyFn    func(adapter.KeyEvent)
	resizeFn func(adapter.ResizeEvent)
}

var _ adapter.UI = (*Gogpu)(nil)

// New returns a Gogpu. term must outlive the UI.
func New(cfg *config.Config, term *terminal.Terminal) *Gogpu {
	f := &Gogpu{
		cfg:  cfg,
		term: term,
		padX: float64(cfg.Window.PaddingX),
		padY: float64(cfg.Window.PaddingY),
	}
	minW, minH := cfg.Window.MinWidth, cfg.Window.MinHeight
	if minW < 1 {
		minW = 400
	}
	if minH < 1 {
		minH = 240
	}
	f.app = gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle(cfg.Window.Title).
		WithSize(cfg.Window.Width, cfg.Window.Height).
		WithMinSize(minW, minH).
		WithResizable(true).
		WithContinuousRender(false))

	term.SetOnTitle(func(s string) {
		f.pendingTitle.Store(&s)
		f.app.RequestRedraw()
	})

	return f
}

// Run starts the gogpu event loop. Blocks until the window closes.
// Must be called from the main goroutine.
func (f *Gogpu) Run() error {
	// OnUpdate runs on the main thread (before renderFrameMultiThread),
	// making it safe to call AppKit APIs like SetTitle.
	f.app.OnUpdate(func(_ float64) {
		if p := f.pendingTitle.Swap(nil); p != nil {
			f.app.SetTitle(*p)
		}
		f.mu.Lock()
		wins := f.windows
		f.mu.Unlock()
		for _, tw := range wins {
			if p := tw.pendingTitle.Swap(nil); p != nil {
				tw.win.SetTitle(*p)
			}
		}
	})
	f.app.OnDraw(f.onDraw)
	f.app.OnResize(f.onWindowResize)
	f.app.OnClose(f.onAppClose)

	// Per-window callbacks are registered after Run() initializes the primary window.
	// Window.SetOn* routes only to the focused window — no EventSource double-dispatch.
	f.app.OnSurfaceAvailable(func() {
		pw := f.app.PrimaryWindow()
		if pw == nil {
			return
		}
		pw.SetOnKeyPress(f.onKeyPress)
		pw.SetOnTextInput(f.onTextInput)
		pw.SetOnScroll(func(e gpucontext.ScrollEvent) {
			sendScrollSeqs(e.DeltaY, f.term.AppCursorKeys(), f.sendKey)
		})
		pw.SetOnPointer(f.onPointerEvent)
	})

	return f.app.Run()

}

// Close requests the window to quit and unblocks Run.
func (f *Gogpu) Close() error {
	f.app.Quit()
	return nil
}

// OnKeyInput registers the callback for key/text input events.
func (f *Gogpu) OnKeyInput(fn func(adapter.KeyEvent)) { f.keyFn = fn }

// OnResize registers the callback for terminal grid size changes.
func (f *Gogpu) OnResize(fn func(adapter.ResizeEvent)) { f.resizeFn = fn }

// RequestRedraw asks gogpu to schedule a new draw frame.
// Thread-safe; called from the PTY reader goroutine.
func (f *Gogpu) RequestRedraw() { f.app.RequestRedraw() }

// ── Primary window callbacks ─────────────────────────────────────────────────

func (f *Gogpu) onDraw(dc *gogpu.Context) {
	w, h := dc.Width(), dc.Height()
	if w <= 0 || h <= 0 {
		return
	}

	if !f.initDone.Load() {
		provider := f.app.GPUContextProvider()
		if provider == nil {
			return
		}
		var err error
		f.canvas, err = ggcanvas.New(provider, w, h)
		if err != nil {
			return
		}
		f.loadFont()
		f.initDone.Store(true)
		f.sendResize(w, h) // send real grid size based on font metrics
	}

	if cw, ch := f.canvas.Size(); cw != w || ch != h {
		_ = f.canvas.Resize(w, h)
		f.sendResize(w, h)
	}

	_ = f.canvas.Draw(func(cc *gg.Context) {
		f.renderTerminal(cc, w, h, f.term, f.sel.snap())
	})
	_ = f.canvas.RenderTo(dc.AsTextureDrawer())
}

func (f *Gogpu) onWindowResize(w, h int) {
	// Clear stuck selection state: on macOS the OS captures the mouseUp event
	// for window-resize drags, so OnMouseRelease never fires and selMouseDown
	// would remain true, activating selection on the next mouse move.
	f.sel.clear()
	f.sendResize(w, h)
}

func (f *Gogpu) onAppClose() {
	f.mu.Lock()
	wins := append([]*termWindow(nil), f.windows...)
	f.mu.Unlock()
	for _, tw := range wins {
		tw.cleanup()
	}
	gg.CloseAccelerator()
}

func (f *Gogpu) onKeyPress(key gpucontext.Key, mods gpucontext.Modifiers) {
	// Cmd+N → open a new terminal window/tab.
	if mods.HasSuper() && !mods.HasShift() && !mods.HasControl() && key == gpucontext.KeyN {
		f.openNewWindow()
		return
	}
	// Cmd+C → copy selection.
	if mods.HasSuper() && !mods.HasShift() && !mods.HasControl() && key == gpucontext.KeyC {
		f.copySelection(f.term, &f.sel)
		return
	}
	// Ctrl+Shift shortcuts (paste, copy, etc.).
	if mods.HasControl() && mods.HasShift() {
		if f.handleShortcut(key, f.term, &f.sel, f.sendKey) {
			return
		}
	}
	seq := encode(key, mods, f.term.AppCursorKeys())
	if seq != nil {
		f.sendKey(seq)
	}
}

func (f *Gogpu) onTextInput(s string) {
	if s != "" {
		f.sendKey([]byte(s))
	}
}

// ── Secondary windows ────────────────────────────────────────────────────────

// termWindow holds per-window state for a secondary terminal session.
type termWindow struct {
	f       *Gogpu
	win     *gogpu.Window
	term    *terminal.Terminal
	canvas  *ggcanvas.Canvas
	write   func([]byte)
	resize  func(uint16, uint16) // PTY resize (nil if not available)
	onClose func()               // PTY cleanup

	pendingTitle atomic.Pointer[string] // set from VTE goroutine, applied in OnUpdate (main thread)
	initDone     atomic.Bool
	sel          windowSelection
}

func (tw *termWindow) cleanup() {
	if tw.onClose != nil {
		tw.onClose()
		tw.onClose = nil
	}
}

func (tw *termWindow) onDraw(dc *gogpu.Context) {
	f := tw.f
	w, h := dc.Width(), dc.Height()
	if w <= 0 || h <= 0 {
		return
	}

	if !tw.initDone.Load() {
		// Reuse the app's shared DeviceProvider — same GPU device.
		provider := f.app.GPUContextProvider()
		if provider == nil {
			return
		}
		var err error
		tw.canvas, err = ggcanvas.New(provider, w, h)
		if err != nil {
			return
		}
		tw.initDone.Store(true)
		// Send initial resize.
		tw.sendResize(w, h)
	}

	if cw, ch := tw.canvas.Size(); cw != w || ch != h {
		_ = tw.canvas.Resize(w, h)
		tw.sendResize(w, h)
	}

	_ = tw.canvas.Draw(func(cc *gg.Context) {
		f.renderTerminal(cc, w, h, tw.term, tw.sel.snap())
	})
	_ = tw.canvas.RenderTo(dc.AsTextureDrawer())
}

func (tw *termWindow) onKeyPress(key gpucontext.Key, mods gpucontext.Modifiers) {
	f := tw.f
	if mods.HasSuper() && !mods.HasShift() && !mods.HasControl() && key == gpucontext.KeyN {
		f.openNewWindow()
		return
	}
	if mods.HasSuper() && !mods.HasShift() && !mods.HasControl() && key == gpucontext.KeyC {
		f.copySelection(tw.term, &tw.sel)
		return
	}
	if mods.HasControl() && mods.HasShift() {
		if f.handleShortcut(key, tw.term, &tw.sel, tw.write) {
			return
		}
	}
	seq := encode(key, mods, tw.term.AppCursorKeys())
	if seq != nil && tw.write != nil {
		tw.write(seq)
	}
}

func (tw *termWindow) onTextInput(s string) {
	if s != "" && tw.write != nil {
		tw.write([]byte(s))
	}
}

func (tw *termWindow) onScroll(e gpucontext.ScrollEvent) {
	writeKey := func(b []byte) {
		if tw.write != nil {
			tw.write(b)
		}
	}
	sendScrollSeqs(e.DeltaY, tw.term.AppCursorKeys(), writeKey)
}

func (tw *termWindow) onResize(w, h int) {
	// Ignore platform resize events until onDraw has run at least once.
	// On macOS a resize event with wrong logical dimensions can arrive before
	// the first draw frame, which would shrink the PTY grid and truncate
	// any content already in the buffer. onDraw handles the initial resize
	// from dc.Width()/dc.Height() which reflect the actual surface size.
	if !tw.initDone.Load() {
		return
	}
	tw.sel.clear()
	tw.sendResize(w, h)
}

func (tw *termWindow) sendResize(w, h int) {
	f := tw.f
	if f.cellW == 0 || f.cellH == 0 {
		return
	}
	cols := uint16((float64(w) - 2*f.padX) / f.cellW)
	rows := uint16((float64(h) - 2*f.padY) / f.cellH)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	tw.term.Resize(int(cols), int(rows))
	if tw.resize != nil {
		tw.resize(cols, rows)
	}
}

// colsRows computes terminal dimensions from logical pixel dimensions and font metrics.
// Falls back to 80×24 if font metrics are not yet loaded.
func (f *Gogpu) colsRows(w, h int) (cols, rows uint16) {
	if f.cellW <= 0 || f.cellH <= 0 {
		return 80, 24
	}
	c := int((float64(w) - 2*f.padX) / f.cellW)
	r := int((float64(h) - 2*f.padY) / f.cellH)
	if c < 1 {
		c = 1
	}
	if r < 1 {
		r = 1
	}
	return uint16(c), uint16(r)
}

// openNewWindow creates a new terminal window (macOS: appears as a new tab).
func (f *Gogpu) openNewWindow() {
	if f.WindowFactory == nil {
		slog.Warn("termizard: openNewWindow called but WindowFactory is nil")
		return
	}

	// Compute the expected terminal size from the configured window dimensions and
	// already-loaded font metrics. The secondary PTY starts at the right size so
	// the first sendResize (from onDraw) is a no-op, not a destructive shrink.
	initCols, initRows := f.colsRows(f.cfg.Window.Width, f.cfg.Window.Height)
	term, write, resize, cleanup := f.WindowFactory(initCols, initRows)
	if term == nil {
		return
	}

	minW, minH := f.cfg.Window.MinWidth, f.cfg.Window.MinHeight
	if minW < 1 {
		minW = 400
	}
	if minH < 1 {
		minH = 240
	}

	win, err := f.app.NewWindow(gogpu.DefaultConfig().
		WithTitle(f.cfg.Window.Title).
		WithSize(f.cfg.Window.Width, f.cfg.Window.Height).
		WithMinSize(minW, minH).
		WithResizable(true).
		WithContinuousRender(false))
	if err != nil {
		slog.Error("termizard: new window", "err", err)
		if cleanup != nil {
			cleanup()
		}
		return
	}

	tw := &termWindow{
		f:       f,
		win:     win,
		term:    term,
		write:   write,
		resize:  resize,
		onClose: cleanup,
	}

	// Store pending title from VTE goroutine; applied on main thread in OnUpdate.
	term.SetOnTitle(func(s string) {
		tw.pendingTitle.Store(&s)
		f.app.RequestRedraw()
	})

	f.mu.Lock()
	f.windows = append(f.windows, tw)
	f.mu.Unlock()

	win.SetOnDraw(tw.onDraw)
	win.SetOnKeyPress(tw.onKeyPress)
	win.SetOnTextInput(tw.onTextInput)
	win.SetOnScroll(tw.onScroll)
	win.SetOnPointer(tw.onPointerEvent)
	win.SetOnResize(tw.onResize)
	win.SetOnClose(func() bool {
		tw.cleanup()
		f.mu.Lock()
		for i, w := range f.windows {
			if w == tw {
				f.windows = append(f.windows[:i], f.windows[i+1:]...)
				break
			}
		}
		f.mu.Unlock()
		return true
	})
}

// ── Shared helpers ───────────────────────────────────────────────────────────

// handleShortcut processes Ctrl+Shift bindings. Returns true if consumed.
func (f *Gogpu) handleShortcut(key gpucontext.Key, term *terminal.Terminal, sel *windowSelection, write func([]byte)) bool {
	for _, kb := range f.cfg.Keybindings {
		if !shortcutMatches(kb, key) {
			continue
		}
		switch kb.Action {
		case "Paste":
			s, err := f.app.ClipboardRead()
			if err != nil || s == "" {
				return true
			}
			if term.BracketedPaste() {
				s = "\x1b[200~" + s + "\x1b[201~"
			}
			write([]byte(s))
			return true
		case "Copy":
			f.copySelection(term, sel)
			return true
		}
	}
	return false
}

func shortcutMatches(kb config.Keybinding, key gpucontext.Key) bool {
	wantKey, ok := namedKey(kb.Key)
	if !ok || wantKey != key {
		return false
	}
	var wantCtrl, wantShift bool
	for _, m := range kb.Mods {
		switch m {
		case "Ctrl", "Control":
			wantCtrl = true
		case "Shift":
			wantShift = true
		}
	}
	return wantCtrl && wantShift
}

func namedKey(name string) (gpucontext.Key, bool) {
	mapping := map[string]gpucontext.Key{
		"a": gpucontext.KeyA, "A": gpucontext.KeyA,
		"b": gpucontext.KeyB, "B": gpucontext.KeyB,
		"c": gpucontext.KeyC, "C": gpucontext.KeyC,
		"d": gpucontext.KeyD, "D": gpucontext.KeyD,
		"e": gpucontext.KeyE, "E": gpucontext.KeyE,
		"f": gpucontext.KeyF, "F": gpucontext.KeyF,
		"g": gpucontext.KeyG, "G": gpucontext.KeyG,
		"h": gpucontext.KeyH, "H": gpucontext.KeyH,
		"i": gpucontext.KeyI, "I": gpucontext.KeyI,
		"j": gpucontext.KeyJ, "J": gpucontext.KeyJ,
		"k": gpucontext.KeyK, "K": gpucontext.KeyK,
		"l": gpucontext.KeyL, "L": gpucontext.KeyL,
		"m": gpucontext.KeyM, "M": gpucontext.KeyM,
		"n": gpucontext.KeyN, "N": gpucontext.KeyN,
		"o": gpucontext.KeyO, "O": gpucontext.KeyO,
		"p": gpucontext.KeyP, "P": gpucontext.KeyP,
		"q": gpucontext.KeyQ, "Q": gpucontext.KeyQ,
		"r": gpucontext.KeyR, "R": gpucontext.KeyR,
		"s": gpucontext.KeyS, "S": gpucontext.KeyS,
		"t": gpucontext.KeyT, "T": gpucontext.KeyT,
		"u": gpucontext.KeyU, "U": gpucontext.KeyU,
		"v": gpucontext.KeyV, "V": gpucontext.KeyV,
		"w": gpucontext.KeyW, "W": gpucontext.KeyW,
		"x": gpucontext.KeyX, "X": gpucontext.KeyX,
		"y": gpucontext.KeyY, "Y": gpucontext.KeyY,
		"z": gpucontext.KeyZ, "Z": gpucontext.KeyZ,
		"Up": gpucontext.KeyUp, "Down": gpucontext.KeyDown,
	}
	k, ok := mapping[name]
	return k, ok
}

func (f *Gogpu) sendKey(data []byte) {
	if f.keyFn != nil {
		f.keyFn(adapter.KeyEvent{Data: data})
	}
}

func (f *Gogpu) sendResize(w, h int) {
	if f.resizeFn == nil || f.cellW == 0 || f.cellH == 0 {
		return
	}
	cols := uint16((float64(w) - 2*f.padX) / f.cellW)
	rows := uint16((float64(h) - 2*f.padY) / f.cellH)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	f.resizeFn(adapter.ResizeEvent{Cols: cols, Rows: rows})
}

// sendScrollSeqs sends 3 cursor-up/down sequences per scroll tick.
func sendScrollSeqs(dy float64, appCursor bool, write func([]byte)) {
	var up, down string
	if appCursor {
		up, down = "\x1bOA", "\x1bOB"
	} else {
		up, down = "\x1b[A", "\x1b[B"
	}
	const ticks = 3
	if dy > 0 {
		for i := 0; i < ticks; i++ {
			write([]byte(up))
		}
	} else if dy < 0 {
		for i := 0; i < ticks; i++ {
			write([]byte(down))
		}
	}
}

// ── Font loading ─────────────────────────────────────────────────────────────

func (f *Gogpu) loadFont() {
	size := f.cfg.Font.Size
	if size <= 0 {
		size = 14
	}

	if f.cfg.Font.Family != "" {
		if src, err := text.NewFontSourceFromFile(f.cfg.Font.Family); err == nil {
			f.fontSrc = src
		}
	}
	if f.fontSrc == nil {
		f.fontSrc = findSystemMonoFont()
	}

	if f.fontSrc != nil {
		f.fontFace = f.fontSrc.Face(size)
		m := f.fontFace.Metrics()
		f.cellW = f.fontFace.Advance("M")
		f.cellH = m.LineHeight()
		f.ascent = m.Ascent
		f.descent = m.Descent
	} else {
		// Rough fixed estimate when no font is found.
		f.cellW = size * 0.6
		f.cellH = size * 1.4
		f.ascent = size * 1.1
		f.descent = size * 0.3
	}
}

func findSystemMonoFont() *text.FontSource {
	candidates := []string{
		homeDir() + "/Library/Fonts/JetBrainsMono-Regular.ttf",
		homeDir() + "/.local/share/fonts/JetBrainsMono-Regular.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/Library/Fonts/Courier New.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/TTF/JetBrainsMono-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/liberation-mono/LiberationMono-Regular.ttf",
		"/usr/share/fonts/truetype/ubuntu/UbuntuMono-R.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if src, err := text.NewFontSourceFromFile(p); err == nil {
				return src
			}
		}
	}
	return nil
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}
