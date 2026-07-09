package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: pprof endpoint is intentional, guarded by --pprof flag
	"os"
	"runtime"
	"runtime/debug"

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

	ui := gogpuui.New(cfg)
	a, err := app.New(cfg, ui)
	if err != nil {
		fail("init", err, logPath)
	}

	logger.Get().Info("starting UI")
	if err := a.Run(); err != nil {
		fail("run", err, logPath)
	}
	logger.Get().Info("UI exited normally")
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
