package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/termizard/termizard/internal/app"
	"github.com/termizard/termizard/internal/util/config"
	"github.com/termizard/termizard/internal/util/logger"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	verbose := flag.Bool("v", false, "enable verbose logging")
	flag.Parse()

	if *verbose {
		logger.Set(
			slog.New(
				slog.NewTextHandler(
					os.Stderr, &slog.HandlerOptions{
						Level: slog.LevelDebug,
					},
				),
			),
		)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "termizard: config: %v\n", err)
		os.Exit(1)
	}

	a, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "termizard: init: %v\n", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "termizard: %v\n", err)
		os.Exit(1)
	}
}
