// Package app wires together PTY, VTE session, terminal model, and UI
// into a single runnable application.
package app

import (
	"os"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/core/vte"
	"github.com/termizard/termizard/internal/ui/gogpu"
	"github.com/termizard/termizard/internal/util/config"
	"github.com/termizard/termizard/internal/util/logger"
)

// App wires all layers and manages goroutine lifecycle.
type App struct {
	cfg *config.Config
	ui  *gogpu.Gogpu
}

// New creates an App, spawning the configured shell in a PTY.
func New(cfg *config.Config) (*App, error) {
	initCols, initRows := cfg.Terminal.InitialCols, cfg.Terminal.InitialRows
	if initCols < 1 {
		initCols = 80
	}
	if initRows < 1 {
		initRows = 24
	}

	term := terminal.New(initCols, initRows, cfg.Scrollback.Lines)
	ui := gogpu.New(cfg, term)

	p, err := openPTY(cfg, uint16(initCols), uint16(initRows))
	if err != nil {
		return nil, err
	}

	sess := wireSession(ui, term, p)
	_ = sess

	a := &App{cfg: cfg, ui: ui}

	ui.WindowFactory = a.newSession

	return a, nil
}

// Run starts the PTY reader goroutine and blocks on the UI event loop.
func (a *App) Run() error {
	return a.ui.Run()
}

// newSession creates a fresh PTY + terminal + VTE session for a new window.
// cols and rows are the expected initial terminal dimensions; the PTY and grid
// are created at this size so no resize is needed immediately after opening.
// Returns nil on error (logged internally).
func (a *App) newSession(cols, rows uint16) (*terminal.Terminal, func([]byte), func(uint16, uint16), func()) {
	if cols < 1 {
		cols = clampUint16(a.cfg.Terminal.InitialCols)
	}
	if rows < 1 {
		rows = clampUint16(a.cfg.Terminal.InitialRows)
	}
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}

	term := terminal.New(int(cols), int(rows), a.cfg.Scrollback.Lines)
	p, err := openPTY(a.cfg, cols, rows)
	if err != nil {
		logger.Get().Error("new window: open pty", "err", err)
		return nil, nil, nil, nil
	}

	sess := vte.NewSession(p, term)
	sess.Notify = a.ui.RequestRedraw

	go func() {
		if err := sess.Run(); err != nil {
			logger.Get().Info("vte session ended", "err", err)
		}
	}()
	go func() { _ = p.Wait() }()

	write := func(b []byte) { _, _ = p.Write(b) }
	resize := func(cols, rows uint16) { _ = p.Resize(cols, rows) }
	cleanup := func() {
		sess.Close()
		_ = p.Close()
	}
	return term, write, resize, cleanup
}

// ── wiring helpers ───────────────────────────────────────────────────────────

func wireSession(ui *gogpu.Gogpu, term *terminal.Terminal, p pty.PTY) *vte.Session {
	sess := vte.NewSession(p, term)
	sess.Notify = ui.RequestRedraw

	ui.OnKeyInput(func(e adapter.KeyEvent) {
		_, _ = p.Write(e.Data)
	})
	ui.OnResize(func(e adapter.ResizeEvent) {
		term.Resize(int(e.Cols), int(e.Rows))
		_ = p.Resize(e.Cols, e.Rows)
	})

	go func() {
		if err := sess.Run(); err != nil {
			logger.Get().Info("vte session ended", "err", err)
		}
		_ = ui.Close()
	}()
	go func() {
		_ = p.Wait()
		_ = ui.Close()
	}()

	return sess
}

func openPTY(cfg *config.Config, cols, rows uint16) (pty.PTY, error) {
	return pty.Open(pty.Config{
		Command: resolveShell(cfg),
		Cols:    cols,
		Rows:    rows,
	})
}

func resolveShell(cfg *config.Config) []string {
	if cfg.Shell.Program != "" {
		return append([]string{cfg.Shell.Program}, cfg.Shell.Args...)
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}

func clampUint16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}
