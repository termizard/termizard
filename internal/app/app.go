// Package app wires together the PTY and UI into a running terminal.
package app

import (
	"sync"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/ui/wails"
	"github.com/termizard/termizard/internal/util/logger"
)

// App holds the wired-up application.
type App struct {
	cfg *config.Config
	ui  adapter.UI
}

// New creates an App. The PTY is opened when the frontend reports its grid size
// via Ready(), so the shell is not resized before the first prompt is drawn.
func New(cfg *config.Config) (*App, error) {
	ui := wails.New(cfg)

	var (
		mu sync.Mutex
		p  pty.PTY
	)

	ui.OnKeyInput(func(e adapter.KeyEvent) {
		mu.Lock()
		ptyInst := p
		mu.Unlock()
		if ptyInst != nil {
			_, _ = ptyInst.Write(e.Data)
		}
	})

	ui.OnResize(func(e adapter.ResizeEvent) {
		mu.Lock()
		ptyInst := p
		mu.Unlock()
		if ptyInst != nil {
			_ = ptyInst.Resize(e.Cols, e.Rows)
		}
	})

	ui.SetPTYStarter(func(cols, rows int) error {
		mu.Lock()
		defer mu.Unlock()
		if p != nil {
			return nil
		}
		if cols < 1 {
			cols = 80
		}
		if rows < 1 {
			rows = 24
		}
		c, r := pty.ClampSize(cols, rows)
		np, err := pty.Open(pty.Config{
			Command: cfg.ShellCommand(),
			Cols:    c,
			Rows:    r,
		})
		if err != nil {
			return err
		}
		p = np
		startPTYLoops(ui, np)
		return nil
	})

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
