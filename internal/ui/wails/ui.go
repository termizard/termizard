// Package wails implements the terminal UI using Wails v3 (native OS webview)
// and xterm.js for terminal rendering.
//
// Data flow:
//
//	PTY output → winSession.write → DispatchWailsEvent("pty:data") → xterm.js
//	xterm.js key input → TerminalService.SendInput → session.keyFn → PTY
//	xterm.js resize → TerminalService.Resize → session.resize → PTY
package wails

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/util/logger"
)

//go:embed all:frontend/dist
var assets embed.FS

const defaultAppName = "termizard"

// XTermTheme holds color values forwarded to xterm.js at startup.
type XTermTheme struct {
	Background    string `json:"background"`
	Foreground    string `json:"foreground"`
	Cursor        string `json:"cursor"`
	Selection     string `json:"selection"`
	Black         string `json:"black"`
	Red           string `json:"red"`
	Green         string `json:"green"`
	Yellow        string `json:"yellow"`
	Blue          string `json:"blue"`
	Magenta       string `json:"magenta"`
	Cyan          string `json:"cyan"`
	White         string `json:"white"`
	BrightBlack   string `json:"brightBlack"`
	BrightRed     string `json:"brightRed"`
	BrightGreen   string `json:"brightGreen"`
	BrightYellow  string `json:"brightYellow"`
	BrightBlue    string `json:"brightBlue"`
	BrightMagenta string `json:"brightMagenta"`
	BrightCyan    string `json:"brightCyan"`
	BrightWhite   string `json:"brightWhite"`
}

// XTermConfig is returned by GetConfig and consumed by the frontend on startup.
type XTermConfig struct {
	FontSize     float64    `json:"fontSize"`
	FontFamily   string     `json:"fontFamily"`
	CursorStyle  string     `json:"cursorStyle"`
	CursorBlink  bool       `json:"cursorBlink"`
	ShowTitleBar bool       `json:"showTitleBar"`
	Theme        XTermTheme `json:"theme"`
}

// winSession holds the PTY and input routing for one terminal window.
type winSession struct {
	win    application.Window
	keyFn  func(adapter.KeyEvent)
	resize func(adapter.ResizeEvent)

	startPTY func(cols, rows int) error

	mu         sync.Mutex
	ready      bool
	ptyStarted bool
	pending    [][]byte
}

func (s *winSession) startPTYOnce(cols, rows int, primaryStart *PTYStarter) {
	s.mu.Lock()
	if s.ptyStarted {
		s.mu.Unlock()
		return
	}
	s.ptyStarted = true
	start := s.startPTY
	s.mu.Unlock()

	if start != nil {
		if err := start(cols, rows); err != nil {
			logger.Get().Error("PTY start failed", "err", err)
		}
		return
	}
	if primaryStart != nil && *primaryStart != nil {
		if err := (*primaryStart)(cols, rows); err != nil {
			logger.Get().Error("primary PTY start failed", "err", err)
		}
	}
}

func (s *winSession) dispatch(data []byte) {
	if len(data) == 0 {
		return
	}
	s.win.DispatchWailsEvent(&application.CustomEvent{
		Name: "pty:data",
		Data: base64.StdEncoding.EncodeToString(data),
	})
}

// write emits raw PTY bytes to this window's xterm.js via a Wails event.
// Output is buffered until the frontend calls Ready().
func (s *winSession) write(data []byte) {
	if len(data) == 0 {
		return
	}
	s.mu.Lock()
	if !s.ready {
		s.pending = append(s.pending, append([]byte(nil), data...))
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.dispatch(data)
}

func (s *winSession) setReady() {
	s.mu.Lock()
	s.ready = true
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()
	for _, chunk := range pending {
		s.dispatch(chunk)
	}
}

// PTYStarter opens the shell PTY once the frontend reports its grid size.
type PTYStarter func(cols, rows int) error

// Each window has its own PTY session; SendInput/Resize are routed via window context.
type TerminalService struct {
	cfg *config.Config
	app *application.App

	// Callbacks for the first window, registered by app.go via adapter.UI before Run().
	primaryKeyFn    atomic.Pointer[func(adapter.KeyEvent)]
	primaryResizeFn atomic.Pointer[func(adapter.ResizeEvent)]
	primaryPTYStart atomic.Pointer[PTYStarter]

	mu       sync.Mutex
	sessions map[uint]*winSession
}

// SetPTYStarter registers the callback that opens the primary PTY when xterm.js
// calls Ready with the actual grid dimensions.
func (s *TerminalService) SetPTYStarter(fn PTYStarter) {
	s.primaryPTYStart.Store(&fn)
}

// GetConfig returns xterm.js configuration for the calling window.
func (s *TerminalService) GetConfig(_ context.Context) XTermConfig {
	return buildXTermConfig(s.cfg)
}

// GetInitialTitle returns the startup working-directory label for the title bar.
func (s *TerminalService) GetInitialTitle(_ context.Context) string {
	return initialTitle()
}

// SetTitle updates the native window title for the calling window.
func (s *TerminalService) SetTitle(ctx context.Context, title string) {
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		w.SetTitle(title)
	}
}

// ToggleMaximize toggles the calling window between maximized and normal size.
func (s *TerminalService) ToggleMaximize(ctx context.Context) {
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		w.ToggleMaximise()
	}
}

// SendInput routes keyboard input from the calling window's xterm.js to its PTY.
func (s *TerminalService) SendInput(ctx context.Context, data string) {
	if sess := s.sessionFromCtx(ctx); sess != nil {
		sess.keyFn(adapter.KeyEvent{Data: []byte(data)})
	}
}

// Resize notifies the PTY of the calling window's new terminal dimensions.
func (s *TerminalService) Resize(ctx context.Context, cols, rows int) {
	if sess := s.sessionFromCtx(ctx); sess != nil {
		c, r := pty.ClampSize(cols, rows)
		sess.resize(adapter.ResizeEvent{Cols: c, Rows: r})
	}
}

// Ready signals that xterm.js is open. The PTY is started at the reported grid
// size so the shell prompt is not corrupted by an early SIGWINCH.
func (s *TerminalService) Ready(ctx context.Context, cols, rows int) {
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return
	}
	sess.startPTYOnce(cols, rows, s.primaryPTYStart.Load())
	sess.setReady()
}

// NewWindow opens a new terminal window in the same process with its own PTY.
func (s *TerminalService) NewWindow(_ context.Context) {
	w := s.app.Window.NewWithOptions(windowOptions(s.cfg))

	sess := &winSession{win: w}
	sess.startPTY = func(cols, rows int) error {
		return s.attachPTY(w, sess, cols, rows)
	}

	s.mu.Lock()
	s.sessions[w.ID()] = sess
	s.mu.Unlock()

	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		delete(s.sessions, w.ID())
		s.mu.Unlock()
	})
}

func (s *TerminalService) attachPTY(w application.Window, sess *winSession, cols, rows int) error {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	c, r := pty.ClampSize(cols, rows)
	p, err := pty.Open(pty.Config{
		Command: s.cfg.ShellCommand(),
		Cols:    c,
		Rows:    r,
	})
	if err != nil {
		logger.Get().Error("PTY open failed", "err", err)
		w.Close()
		return err
	}

	sess.keyFn = func(e adapter.KeyEvent) { _, _ = p.Write(e.Data) }
	sess.resize = func(e adapter.ResizeEvent) { _ = p.Resize(e.Cols, e.Rows) }

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				sess.write(buf[:n])
			}
			if err != nil {
				w.Close()
				return
			}
		}
	}()

	go func() {
		_ = p.Wait()
		w.Close()
	}()

	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		_ = p.Close()
	})

	return nil
}

func (s *TerminalService) sessionFromCtx(ctx context.Context) *winSession {
	w, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok {
		return nil
	}
	s.mu.Lock()
	sess := s.sessions[w.ID()]
	s.mu.Unlock()
	return sess
}

// UI implements adapter.UI for the primary (first) terminal window.
type UI struct {
	cfg     *config.Config
	svc     *TerminalService
	primary atomic.Pointer[winSession]

	earlyMu      sync.Mutex
	earlyPending [][]byte
}

// New returns a UI for the given config.
func New(cfg *config.Config) *UI {
	return &UI{
		cfg: cfg,
		svc: &TerminalService{
			cfg:      cfg,
			sessions: make(map[uint]*winSession),
		},
	}
}

// SetPTYStarter registers the callback that opens the primary shell PTY.
func (u *UI) SetPTYStarter(fn PTYStarter) { u.svc.SetPTYStarter(fn) }

// OnKeyInput registers the key-input callback for the primary window.
func (u *UI) OnKeyInput(fn func(adapter.KeyEvent)) { u.svc.primaryKeyFn.Store(&fn) }

// OnResize registers the resize callback for the primary window.
func (u *UI) OnResize(fn func(adapter.ResizeEvent)) { u.svc.primaryResizeFn.Store(&fn) }

// RequestRedraw is a no-op: xterm.js renders on every Write call.
func (u *UI) RequestRedraw() {}

// Write forwards raw PTY bytes to the primary window's xterm.js.
func (u *UI) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	p := u.primary.Load()
	if p == nil {
		u.earlyMu.Lock()
		u.earlyPending = append(u.earlyPending, append([]byte(nil), data...))
		u.earlyMu.Unlock()
		return len(data), nil
	}
	p.write(data)
	return len(data), nil
}

// Close signals the primary window to close.
func (u *UI) Close() error {
	if p := u.primary.Load(); p != nil {
		p.win.Close()
	}
	return nil
}

// Run starts the Wails v3 event loop. Blocks until all windows close.
func (u *UI) Run() error {
	app := application.New(application.Options{
		Name: defaultAppName,
		Services: []application.Service{
			application.NewService(u.svc),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	u.svc.app = app

	win := app.Window.NewWithOptions(windowOptions(u.cfg))

	// Wire the primary session using callbacks registered before Run().
	keyFn := func(e adapter.KeyEvent) {}
	resizeFn := func(e adapter.ResizeEvent) {}
	if p := u.svc.primaryKeyFn.Load(); p != nil {
		keyFn = *p
	}
	if p := u.svc.primaryResizeFn.Load(); p != nil {
		resizeFn = *p
	}
	sess := &winSession{win: win, keyFn: keyFn, resize: resizeFn}
	u.primary.Store(sess)

	u.svc.mu.Lock()
	u.svc.sessions[win.ID()] = sess
	u.svc.mu.Unlock()

	u.earlyMu.Lock()
	early := u.earlyPending
	u.earlyPending = nil
	u.earlyMu.Unlock()
	for _, chunk := range early {
		sess.write(chunk)
	}

	return app.Run()
}

func windowOptions(cfg *config.Config) application.WebviewWindowOptions {
	title := cfg.Window.Title
	if title == "" {
		title = defaultAppName
	}
	w := cfg.Window.Width
	if w <= 0 {
		w = 1200
	}
	h := cfg.Window.Height
	if h <= 0 {
		h = 800
	}
	minW := cfg.Window.MinWidth
	if minW <= 0 {
		minW = 400
	}
	minH := cfg.Window.MinHeight
	if minH <= 0 {
		minH = 240
	}
	titleBarHeight := 0
	if cfg.Window.ShowTitleBar {
		titleBarHeight = 38
	}
	return application.WebviewWindowOptions{
		Title:            title,
		Width:            w,
		Height:           h,
		MinWidth:         minW,
		MinHeight:        minH,
		BackgroundColour: application.RGBA{Red: 30, Green: 31, Blue: 34, Alpha: 255},
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: titleBarHeight,
		},
	}
}

func initialTitle() string {
	// Mirror pty.resolveDir: the shell starts in $HOME, so the initial title is ~.
	if os.Getenv("HOME") != "" {
		return "~"
	}
	if _, err := os.UserHomeDir(); err == nil {
		return "~"
	}
	return defaultAppName
}

func buildXTermConfig(cfg *config.Config) XTermConfig {
	c := cfg.Colors
	f := cfg.Font
	cur := cfg.Cursor
	fontSize := f.Size
	if fontSize <= 0 {
		fontSize = 14
	}
	cursorBlink := cur.Blink
	if cur.Shape == "" && !cur.Blink {
		cursorBlink = true // default on for fresh installs
	}
	return XTermConfig{
		FontSize:     fontSize,
		FontFamily:   orDefault(f.Family, `"Menlo", "SF Mono", "JetBrains Mono", "Monaco", monospace`),
		CursorStyle:  orDefault(cur.Shape, "block"),
		CursorBlink:  cursorBlink,
		ShowTitleBar: cfg.Window.ShowTitleBar,
		Theme: XTermTheme{
			Background:    orDefault(c.Background, "rgba(30, 31, 34, 0.82)"),
			Foreground:    orDefault(c.Foreground, "#BCBEC4"),
			Cursor:        orDefault(c.Cursor, "#FFCC66"),
			Selection:     orDefault(c.Selection, "rgba(33, 66, 131, 0.85)"),
			Black:         orDefault(c.ANSI.Black, "#2B2B2B"),
			Red:           orDefault(c.ANSI.Red, "#FF6B68"),
			Green:         orDefault(c.ANSI.Green, "#A8C023"),
			Yellow:        orDefault(c.ANSI.Yellow, "#FFC66D"),
			Blue:          orDefault(c.ANSI.Blue, "#5394EC"),
			Magenta:       orDefault(c.ANSI.Magenta, "#9876AA"),
			Cyan:          orDefault(c.ANSI.Cyan, "#299999"),
			White:         orDefault(c.ANSI.White, "#A9B7C6"),
			BrightBlack:   orDefault(c.ANSI.BrightBlack, "#555555"),
			BrightRed:     orDefault(c.ANSI.BrightRed, "#FF8782"),
			BrightGreen:   orDefault(c.ANSI.BrightGreen, "#B8E986"),
			BrightYellow:  orDefault(c.ANSI.BrightYellow, "#FFD37F"),
			BrightBlue:    orDefault(c.ANSI.BrightBlue, "#6BAFFF"),
			BrightMagenta: orDefault(c.ANSI.BrightMagenta, "#C198CB"),
			BrightCyan:    orDefault(c.ANSI.BrightCyan, "#3DDAD7"),
			BrightWhite:   orDefault(c.ANSI.BrightWhite, "#FFFFFF"),
		},
	}
}

// GetConfigJSON returns the xterm config as JSON (kept for potential dev tooling).
func (s *TerminalService) GetConfigJSON(_ context.Context) string {
	b, _ := json.Marshal(buildXTermConfig(s.cfg))
	return string(b)
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

var _ adapter.UI = (*UI)(nil)
