//go:build windows

package main

import (
	"github.com/termizard/termizard/internal/app"
	"github.com/termizard/termizard/internal/config"
	gogpuui "github.com/termizard/termizard/internal/ui/gogpu"
)

// softwareRetry on Windows does an in-process retry with a new gogpu App using
// the software renderer. Windows does not have the Wayland focus problem that
// makes in-process retry unsuitable on Linux.
func softwareRetry(cfg *config.Config, logPath string) {
	ui2 := gogpuui.New(cfg)
	a2, err := app.New(cfg, ui2)
	if err != nil {
		fail("init-sw", err, logPath)
		return
	}
	if err := a2.Run(); err != nil {
		fail("run-sw", err, logPath)
	}
}
