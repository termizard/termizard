package main

import (
	"log"

	"github.com/termizard/termizard/internal/app"
	"github.com/termizard/termizard/internal/pty"
)

func main() {
	// Getting the path to the shell
	shell := pty.GetDefaultShell()

	termInstance, err := pty.StartPTY(shell)
	if err != nil {
		log.Fatal(err)
	}

	application := app.NewApp(termInstance)

	if err := application.Run(); err != nil {
		log.Fatal(err)
	}
}
