// Package app wires together the PTY and UI into a running terminal.
package app

import (
	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/util/logger"
)

// App holds the wired-up application.
type App struct {
	cfg *config.Config
	ui  adapter.UI
}

// surfaceReadyUI can defer PTY start until the GPU surface exists.
type surfaceReadyUI interface {
	adapter.UI
	OnSurfaceReady(fn func())
}

// New creates an App using the provided UI backend.
// The PTY is opened with the configured initial dimensions and connected to ui.
func New(cfg *config.Config, ui adapter.UI) (*App, error) {
	cols := cfg.Terminal.InitialCols
	rows := cfg.Terminal.InitialRows
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	c, r := pty.ClampSize(cols, rows)
	p, err := pty.Open(pty.Config{
		Command: cfg.ShellCommand(),
		Cols:    c,
		Rows:    r,
	})
	if err != nil {
		return nil, err
	}

	ui.OnKeyInput(func(e adapter.KeyEvent) {
		_, _ = p.Write(e.Data)
	})
	ui.OnResize(func(e adapter.ResizeEvent) {
		_ = p.Resize(e.Cols, e.Rows)
	})

	if sru, ok := ui.(surfaceReadyUI); ok {
		sru.OnSurfaceReady(func() { startPTYLoops(ui, p) })
	} else {
		startPTYLoops(ui, p)
	}
	return &App{cfg: cfg, ui: ui}, nil
}

// Run starts the UI event loop. Blocks until the window closes.
func (a *App) Run() error {
	return a.ui.Run()
}

func startPTYLoops(ui adapter.UI, p pty.PTY) {
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				if _, werr := ui.Write(buf[:n]); werr != nil {
					logger.Get().Debug("ui write error", "err", werr)
				}
			}
			if err != nil {
				_ = ui.Close()
				return
			}
		}
	}()

	go func() {
		_ = p.Wait()
		_ = ui.Close()
	}()
}
