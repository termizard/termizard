// Package wails implements the terminal UI using Wails v3 (native OS webview)
// and xterm.js for terminal rendering.
//
// Data flow:
//
//	PTY output → tabSession.write → DispatchWailsEvent("pty:data") → xterm.js
//	xterm.js key input → TerminalService.SendInput → tab.keyFn → PTY
//	xterm.js resize → TerminalService.Resize → tab.resize → PTY
package wails

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/termizard/termizard/frontend"
	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/util/logger"
)

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
	FontSize      float64          `json:"fontSize"`
	FontFamily    string           `json:"fontFamily"`
	CursorStyle   string           `json:"cursorStyle"`
	CursorBlink   bool             `json:"cursorBlink"`
	ShowTitleBar  bool             `json:"showTitleBar"`
	TitlePosition string           `json:"titlePosition"`
	PaddingX      int              `json:"paddingX"`
	PaddingY      int              `json:"paddingY"`
	Theme         XTermTheme       `json:"theme"`
	Keybindings   []KeyBindingJSON `json:"keybindings"`
}

// KeyBindingJSON is a keyboard shortcut exposed to the frontend.
type KeyBindingJSON struct {
	Key    string   `json:"key"`
	Mods   []string `json:"mods"`
	Action string   `json:"action"`
}

// TabInfo describes one tab for the frontend tab bar.
type TabInfo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// TabsResponse is returned by GetTabs.
type TabsResponse struct {
	Enabled        bool      `json:"enabled"`
	Label          string    `json:"label"`
	ShowNewButton  bool      `json:"showNewButton"`
	ShowWhenSingle bool      `json:"showWhenSingle"`
	Items          []TabInfo `json:"items"`
}

type ptyDataPayload struct {
	TabID int    `json:"tabId"`
	Data  string `json:"data"`
}

type tabSession struct {
	keyFn    func(adapter.KeyEvent)
	resize   func(adapter.ResizeEvent)
	startPTY func(cols, rows int) error
	closeFn  func() error

	mu         sync.Mutex
	ready      bool
	ptyStarted bool
	pending    [][]byte
}

func (t *tabSession) startPTYOnce(cols, rows int, primaryStart *PTYStarter) {
	t.mu.Lock()
	if t.ptyStarted {
		t.mu.Unlock()
		return
	}
	t.ptyStarted = true
	start := t.startPTY
	t.mu.Unlock()

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

func (t *tabSession) dispatch(win application.Window, tabID int, data []byte) {
	if len(data) == 0 {
		return
	}
	payload, err := json.Marshal(ptyDataPayload{
		TabID: tabID,
		Data:  base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return
	}
	win.DispatchWailsEvent(&application.CustomEvent{
		Name: "pty:data",
		Data: string(payload),
	})
}

func (t *tabSession) write(win application.Window, tabID int, data []byte) {
	if len(data) == 0 {
		return
	}
	t.mu.Lock()
	if !t.ready {
		t.pending = append(t.pending, append([]byte(nil), data...))
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	t.dispatch(win, tabID, data)
}

func (t *tabSession) setReady(win application.Window, tabID int) {
	t.mu.Lock()
	t.ready = true
	pending := t.pending
	t.pending = nil
	t.mu.Unlock()
	for _, chunk := range pending {
		t.dispatch(win, tabID, chunk)
	}
}

// winSession holds PTY routing for one Wails window (one or more tabs).
type winSession struct {
	win       application.Window
	tabs      map[int]*tabSession
	nextTabID int
	mu        sync.RWMutex
}

func newWinSession(win application.Window) *winSession {
	return &winSession{win: win, tabs: make(map[int]*tabSession), nextTabID: 1}
}

func (s *winSession) tab(tabID int) *tabSession {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tabs[tabID]
}

func (s *winSession) ensureTab(tabID int) *tabSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tabs[tabID]; t != nil {
		return t
	}
	t := &tabSession{}
	s.tabs[tabID] = t
	return t
}

func (s *winSession) tabCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tabs)
}

func (s *winSession) tabIDs() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]int, 0, len(s.tabs))
	for id := range s.tabs {
		ids = append(ids, id)
	}
	return ids
}

func (s *winSession) write(tabID int, data []byte) {
	if t := s.tab(tabID); t != nil {
		t.write(s.win, tabID, data)
	}
}

// PTYStarter opens the shell PTY once the frontend reports its grid size.
type PTYStarter func(cols, rows int) error

// TerminalService exposes Wails methods to the frontend.
type TerminalService struct {
	cfg *config.Config
	app *application.App

	primaryKeyFn    atomic.Pointer[func(adapter.KeyEvent)]
	primaryResizeFn atomic.Pointer[func(adapter.ResizeEvent)]
	primaryPTYStart atomic.Pointer[PTYStarter]

	mu       sync.Mutex
	sessions map[uint]*winSession
}

func (s *TerminalService) SetPTYStarter(fn PTYStarter) {
	s.primaryPTYStart.Store(&fn)
}

func (s *TerminalService) GetConfig(_ context.Context) XTermConfig {
	return buildXTermConfig(s.cfg)
}

func (s *TerminalService) GetTabs(ctx context.Context) TabsResponse {
	label := config.TabLabelPath
	showNew := true
	showSingle := false
	if s.cfg.Tabs.Enabled {
		label = s.cfg.Tabs.Label
		showNew = s.cfg.Tabs.ShowNewButton
		showSingle = s.cfg.Tabs.ShowWhenSingle
	}
	resp := TabsResponse{
		Enabled:        s.cfg.Tabs.Enabled,
		Label:          label,
		ShowNewButton:  showNew,
		ShowWhenSingle: showSingle,
		Items:          []TabInfo{{ID: 0}},
	}
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return resp
	}
	ids := sess.tabIDs()
	sort.Ints(ids)
	items := make([]TabInfo, len(ids))
	for i, id := range ids {
		items[i] = TabInfo{ID: id}
	}
	resp.Items = items
	return resp
}

func (s *TerminalService) GetInitialTitle(_ context.Context) string {
	return initialTitle()
}

func (s *TerminalService) SetTitle(ctx context.Context, title string) {
	if s.cfg.Window.ShowTitleBar {
		// Custom in-app title bar renders the path; avoid macOS native title near traffic lights.
		return
	}
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		w.SetTitle(title)
	}
}

func (s *TerminalService) ToggleMaximize(ctx context.Context) {
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		w.ToggleMaximise()
	}
}

func (s *TerminalService) SendInput(ctx context.Context, tabID int, data string) {
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return
	}
	tab := sess.tab(tabID)
	if tab == nil || tab.keyFn == nil {
		return
	}
	tab.keyFn(adapter.KeyEvent{Data: []byte(data)})
}

func (s *TerminalService) Resize(ctx context.Context, tabID, cols, rows int) {
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return
	}
	tab := sess.tab(tabID)
	if tab == nil || tab.resize == nil {
		return
	}
	c, r := pty.ClampSize(cols, rows)
	tab.resize(adapter.ResizeEvent{Cols: c, Rows: r})
}

func (s *TerminalService) Ready(ctx context.Context, tabID, cols, rows int) {
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return
	}
	tab := sess.ensureTab(tabID)
	tab.startPTYOnce(cols, rows, s.primaryPTYStart.Load())
	tab.setReady(sess.win, tabID)
}

// NewTab allocates a new tab slot in the current window.
func (s *TerminalService) NewTab(ctx context.Context) (TabInfo, error) {
	if !s.cfg.Tabs.Enabled {
		return TabInfo{}, errors.New("tabs disabled")
	}
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return TabInfo{}, errors.New("no window session")
	}
	sess.mu.Lock()
	tabID := sess.nextTabID
	sess.nextTabID++
	sess.mu.Unlock()

	tab := &tabSession{}
	tab.startPTY = func(cols, rows int) error {
		return s.attachPTY(sess.win, sess, tabID, s.cfg.ShellCommand(), "", cols, rows)
	}
	sess.mu.Lock()
	sess.tabs[tabID] = tab
	sess.mu.Unlock()
	return TabInfo{ID: tabID}, nil
}

// CloseTab closes a tab and its PTY. The last tab cannot be closed.
func (s *TerminalService) CloseTab(ctx context.Context, tabID int) error {
	if !s.cfg.Tabs.Enabled {
		return errors.New("tabs disabled")
	}
	sess := s.sessionFromCtx(ctx)
	if sess == nil {
		return errors.New("no window session")
	}
	if sess.tabCount() <= 1 {
		return errors.New("cannot close last tab")
	}
	tab := sess.tab(tabID)
	if tab == nil {
		return errors.New("tab not found")
	}
	if tab.closeFn != nil {
		_ = tab.closeFn()
	}
	sess.mu.Lock()
	delete(sess.tabs, tabID)
	sess.mu.Unlock()
	return nil
}

func (s *TerminalService) NewWindow(_ context.Context) {
	w := s.app.Window.NewWithOptions(windowOptions(s.cfg))
	sess := newWinSession(w)
	tab0 := &tabSession{}
	tab0.startPTY = func(cols, rows int) error {
		return s.attachPTY(w, sess, 0, s.cfg.ShellCommand(), "", cols, rows)
	}
	sess.tabs[0] = tab0

	s.mu.Lock()
	s.sessions[w.ID()] = sess
	s.mu.Unlock()

	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		delete(s.sessions, w.ID())
		s.mu.Unlock()
	})
}

func (s *TerminalService) attachPTY(
	w application.Window,
	sess *winSession,
	tabID int,
	command []string,
	dir string,
	cols, rows int,
) error {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	c, r := pty.ClampSize(cols, rows)
	env, err := s.cfg.ShellEnvironment(nil)
	if err != nil {
		logger.Get().Warn("shell env setup failed", "err", err)
		env = nil
	}
	cfg := pty.Config{
		Command: command,
		Cols:    c,
		Rows:    r,
		Env:     env,
	}
	if dir != "" {
		cfg.Dir = dir
	}
	p, err := pty.Open(cfg)
	if err != nil {
		logger.Get().Error("PTY open failed", "err", err)
		if sess.tabCount() <= 1 {
			w.Close()
		}
		return err
	}

	tab := sess.ensureTab(tabID)
	tab.keyFn = func(e adapter.KeyEvent) { _, _ = p.Write(e.Data) }
	tab.resize = func(e adapter.ResizeEvent) { _ = p.Resize(e.Cols, e.Rows) }
	tab.closeFn = func() error { return p.Close() }

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				sess.write(tabID, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	go func() {
		_ = p.Wait()
		s.handlePTYExit(w, sess, tabID)
	}()

	return nil
}

func (s *TerminalService) handlePTYExit(w application.Window, sess *winSession, tabID int) {
	sess.mu.Lock()
	delete(sess.tabs, tabID)
	remaining := len(sess.tabs)
	sess.mu.Unlock()

	if remaining == 0 {
		w.Close()
		return
	}

	payload, err := json.Marshal(map[string]int{"tabId": tabID})
	if err != nil {
		return
	}
	sess.win.DispatchWailsEvent(&application.CustomEvent{
		Name: "tab:closed",
		Data: string(payload),
	})
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

func (s *TerminalService) initPrimarySession(win application.Window, keyFn func(adapter.KeyEvent), resizeFn func(adapter.ResizeEvent)) *winSession {
	sess := newWinSession(win)
	tab0 := &tabSession{}
	if s.cfg.Tabs.Enabled {
		tab0.startPTY = func(cols, rows int) error {
			return s.attachPTY(win, sess, 0, s.cfg.ShellCommand(), "", cols, rows)
		}
	} else {
		tab0.keyFn = keyFn
		tab0.resize = resizeFn
	}
	sess.tabs[0] = tab0
	return sess
}

type UI struct {
	cfg     *config.Config
	svc     *TerminalService
	primary atomic.Pointer[winSession]

	earlyMu      sync.Mutex
	earlyPending [][]byte
}

func New(cfg *config.Config) *UI {
	return &UI{
		cfg: cfg,
		svc: &TerminalService{
			cfg:      cfg,
			sessions: make(map[uint]*winSession),
		},
	}
}

func (u *UI) SetPTYStarter(fn PTYStarter) { u.svc.SetPTYStarter(fn) }

func (u *UI) OnKeyInput(fn func(adapter.KeyEvent)) { u.svc.primaryKeyFn.Store(&fn) }

func (u *UI) OnResize(fn func(adapter.ResizeEvent)) { u.svc.primaryResizeFn.Store(&fn) }

func (u *UI) RequestRedraw() {}

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
	p.write(0, data)
	return len(data), nil
}

func (u *UI) Close() error {
	if p := u.primary.Load(); p != nil {
		p.win.Close()
	}
	return nil
}

func (u *UI) Run() error {
	app := application.New(application.Options{
		Name: defaultAppName,
		Services: []application.Service{
			application.NewService(u.svc),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(frontend.Dist),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	u.svc.app = app

	win := app.Window.NewWithOptions(windowOptions(u.cfg))

	keyFn := func(e adapter.KeyEvent) {}
	resizeFn := func(e adapter.ResizeEvent) {}
	if p := u.svc.primaryKeyFn.Load(); p != nil {
		keyFn = *p
	}
	if p := u.svc.primaryResizeFn.Load(); p != nil {
		resizeFn = *p
	}

	sess := u.svc.initPrimarySession(win, keyFn, resizeFn)
	u.primary.Store(sess)

	u.svc.mu.Lock()
	u.svc.sessions[win.ID()] = sess
	u.svc.mu.Unlock()

	u.earlyMu.Lock()
	early := u.earlyPending
	u.earlyPending = nil
	u.earlyMu.Unlock()
	for _, chunk := range early {
		sess.write(0, chunk)
	}

	return app.Run()
}

// macOSTrafficLightZone is the native caption height where traffic lights are vertically
// centered. InvisibleTitleBarHeight must match this — not the full custom title bar CSS height.
const macOSTrafficLightZone = 28

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
	nativeTitle := title
	if cfg.Window.ShowTitleBar {
		titleBarHeight = macOSTrafficLightZone
		nativeTitle = defaultAppName
	}
	return application.WebviewWindowOptions{
		Title:            nativeTitle,
		Width:            w,
		Height:           h,
		MinWidth:         minW,
		MinHeight:        minH,
		BackgroundColour: application.RGBA{Red: 30, Green: 31, Blue: 34, Alpha: 255},
		Mac: application.MacWindow{
			// Hidden (no NSToolbar): traffic lights stay vertically centered in the caption
			// zone. HiddenInset + toolbar pins them to the bottom of a taller toolbar strip.
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: titleBarHeight,
		},
	}
}

func initialTitle() string {
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
		cursorBlink = true
	}
	paddingX := cfg.Window.PaddingX
	paddingY := cfg.Window.PaddingY
	if paddingX <= 0 {
		paddingX = 6
	}
	if paddingY <= 0 {
		paddingY = 6
	}
	return XTermConfig{
		FontSize:      fontSize,
		FontFamily:    orDefault(f.Family, `"Menlo", "SF Mono", "JetBrains Mono", "Monaco", monospace`),
		CursorStyle:   orDefault(cur.Shape, "block"),
		CursorBlink:   cursorBlink,
		ShowTitleBar:  cfg.Window.ShowTitleBar,
		TitlePosition: orDefault(cfg.Window.TitlePosition, config.TitleAlignCenter),
		PaddingX:      paddingX,
		PaddingY:      paddingY,
		Theme: XTermTheme{
			Background:    orDefault(c.Background, "rgba(30, 31, 34, 0.82)"),
			Foreground:    orDefault(c.Foreground, "#BCBEC4"),
			Cursor:        orDefault(c.Cursor, "#BCBEC4"),
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
		Keybindings: buildKeybindingsJSON(cfg.Keybindings),
	}
}

func buildKeybindingsJSON(bindings []config.Keybinding) []KeyBindingJSON {
	out := make([]KeyBindingJSON, 0, len(bindings))
	for _, kb := range bindings {
		mods := kb.Mods
		if mods == nil {
			mods = []string{}
		}
		out = append(out, KeyBindingJSON{
			Key:    kb.Key,
			Mods:   append([]string(nil), mods...),
			Action: kb.Action,
		})
	}
	return out
}

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
