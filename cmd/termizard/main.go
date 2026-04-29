package main

import (
	"log"
	"log/slog"

	"github.com/termizard/termizard/internal/app"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/pty"
)

func main() {
	cfg := config.MustLoad()
	slog.Info("cfg loaded", slog.Any("cfg", cfg))
	// Getting the path to the shell
	shell := pty.GetDefaultShell()

	termInstance, err := pty.StartPTY(shell)
	if err != nil {
		log.Fatal(err)
	}

	application := app.NewApp(termInstance, cfg.Terminal.Rows, cfg.Terminal.Cols)

	if err := application.Run(); err != nil {
		log.Fatal(err)
	}
}
