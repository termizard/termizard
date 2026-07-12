//go:build !windows

package main

import (
	"os"
	"syscall"

	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/util/logger"
)

// softwareRetry re-executes the binary with GOGPU_GRAPHICS_API=software so the
// new process gets proper Wayland keyboard focus from the compositor. An in-process
// retry keeps the same process and the compositor does not grant it fresh focus,
// leaving all input broken on Wayland.
func softwareRetry(cfg *config.Config, logPath string) {
	exe, err := os.Executable()
	if err != nil {
		fail("reexec-lookup", err, logPath)
		return
	}
	logger.Get().Warn("re-execing with software renderer", "exe", exe)
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil { //nolint:gosec // G204: exe is os.Executable(), not user-controlled input
		fail("reexec", err, logPath)
	}
}
