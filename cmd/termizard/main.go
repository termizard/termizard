package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: pprof endpoint is intentional, guarded by --pprof flag
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/termizard/termizard/internal/app"
	"github.com/termizard/termizard/internal/config"
	gogpuui "github.com/termizard/termizard/internal/ui/gogpu"
	"github.com/termizard/termizard/internal/util/logger"
	"github.com/termizard/termizard/internal/util/winmsg"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	verbose := flag.Bool("v", false, "enable verbose logging")
	pprofAddr := flag.String("pprof", "", "enable pprof HTTP server on this address (e.g. :6060)")
	flag.Parse()

	if *pprofAddr != "" {
		go func() {
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil { //nolint:gosec // G114: pprof addr is user-supplied via --pprof flag
				fmt.Fprintf(os.Stderr, "pprof server: %v\n", err)
			}
		}()
	}

	logPath, err := logger.Setup(*verbose)
	if err != nil {
		fail("logging", err, "")
	}
	defer logger.Close()
	defer func() {
		if r := recover(); r != nil {
			logger.Get().Error("panic recovered", "msg", fmt.Sprintf("%v", r), "stack", string(debug.Stack()))
			fail("panic", fmt.Errorf("%v\n%s", r, debug.Stack()), logPath)
		}
	}()

	logger.Get().Info("startup", "config", *cfgPath, "verbose", *verbose)

	if err := config.EnsureDefaultFile(*cfgPath); err != nil {
		fail("config", err, logPath)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fail("config", err, logPath)
	}

	logger.Get().Debug("creating UI")
	ui := gogpuui.New(cfg)
	logger.Get().Debug("UI created, creating app")
	a, err := app.New(cfg, ui)
	logger.Get().Debug("app created", "err", err)
	if err != nil {
		logger.Get().Error("app init failed", "err", err)
		fail("init", err, logPath)
	}

	logger.Get().Info("starting UI")
	if err := a.Run(); err != nil {
		runFailed(a, cfg, err, logPath)
		return
	}
	logger.Get().Info("UI exited normally")
}

// runFailed handles a Run() error: if it looks like a GPU init failure and no
// backend was forced, retry transparently with the software rasteriser.
// Mirrors Rio terminal's CPU-fallback behavior on unsupported GPU drivers.
func runFailed(a *app.App, cfg *config.Config, runErr error, logPath string) {
	_ = a.Close()
	if !isGPUError(runErr) || os.Getenv("GOGPU_GRAPHICS_API") != "" {
		fail("run", runErr, logPath)
		return
	}
	logger.Get().Warn("GPU init failed, retrying with software renderer", "err", runErr)
	if err := os.Setenv("GOGPU_GRAPHICS_API", "software"); err != nil {
		fail("run", runErr, logPath)
		return
	}
	softwareRetry(cfg, logPath)
}

// isGPUError reports whether err looks like a wgpu/GPU initialisation failure
// that can be retried with a software renderer.
func isGPUError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "failed to request device") ||
		strings.Contains(s, "failed to request adapter") ||
		strings.Contains(s, "failed to open device")
}

func fail(stage string, err error, logPath string) {
	msg := fmt.Sprintf("termizard: %s: %v", stage, err)
	if runtime.GOOS == "windows" {
		if logPath != "" {
			msg += fmt.Sprintf("\n\nLog: %s", logPath)
		}
		winmsg.Error("termizard", msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	os.Exit(1)
}
