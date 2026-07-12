// Package app wires together the PTY and UI into a running terminal.
package app

import (
	"sync"
	"sync/atomic"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/util/logger"
)

// App holds the wired-up application.
type App struct {
	cfg          *config.Config
	ui           adapter.UI
	p            pty.PTY
	surfaceReady atomic.Bool
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
		if _, err := p.Write(e.Data); err != nil {
			logger.Get().Warn("pty input write failed", "bytes", len(e.Data), "err", err)
		}
	})
	ui.OnResize(func(e adapter.ResizeEvent) {
		_ = p.Resize(e.Cols, e.Rows)
	})

	a := &App{cfg: cfg, ui: ui, p: p}

	if sru, ok := ui.(surfaceReadyUI); ok {
		sru.OnSurfaceReady(func() {
			logger.Get().Debug("OnSurfaceReady fired, starting PTY loops")
			a.surfaceReady.Store(true)
			startPTYLoops(ui, p)
		})
	} else {
		logger.Get().Debug("no surfaceReadyUI, starting PTY loops immediately")
		a.surfaceReady.Store(true)
		startPTYLoops(ui, p)
	}
	return a, nil
}

// Run starts the UI event loop. Blocks until the window closes.
func (a *App) Run() error {
	return a.ui.Run()
}

// Close terminates the PTY child process. It is only safe to call when the
// GPU surface never became ready, meaning the PTY read loops were never
// started (e.g. GPU initialisation failed before OnSurfaceReady fired).
// If the surface was already ready, Close is a no-op to avoid racing the
// running PTY goroutines.
func (a *App) Close() error {
	if !a.surfaceReady.Load() && a.p != nil {
		return a.p.Close()
	}
	return nil
}

func startPTYLoops(ui adapter.UI, p pty.PTY) {
	// sync.Once ensures ui.Close is called exactly once even when both the
	// read-error path and the process-exit path fire concurrently.
	var once sync.Once
	closeUI := func() { once.Do(func() { _ = ui.Close() }) }

	go func() {
		logger.Get().Debug("pty read loop: started")
		buf := make([]byte, 32*1024)
		for {
			logger.Get().Debug("pty read loop: calling Read")
			n, err := p.Read(buf)
			logger.Get().Debug("pty read loop: Read returned", "n", n, "err", err)
			if n > 0 {
				if _, werr := ui.Write(buf[:n]); werr != nil {
					logger.Get().Debug("ui write error", "err", werr)
				}
			}
			if err != nil {
				closeUI()
				return
			}
		}
	}()

	go func() {
		_ = p.Wait()
		closeUI()
	}()
}
