package app

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/termizard/termizard/internal/pty"
	"golang.org/x/term"
)

// App represents the main terminal manager application.
type App struct {
	terminal pty.PTY
	rows     int
	cols     int
}

// NewApp creates a new instance of the application.
func NewApp(ptyInstance pty.PTY, rows, cols int) *App {
	// Оборачиваем PTY в эмулятор
	emulated := pty.NewEmulatedPTY(ptyInstance, rows, cols)
	return &App{
		terminal: emulated,
		rows:     rows,
		cols:     cols,
	}
}

// Run Runs the main loop of the application, handling input and output.
func (a *App) Run() error {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		// Переводим терминал в сырой режим (raw mode)
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to make terminal raw: %w", err)
		}
		defer func() { _ = term.Restore(fd, oldState) }()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Reading from PTY to console
	go func() {
		_, _ = io.Copy(os.Stdout, a.terminal)
	}()

	// We write from the console to PTY
	go func() {
		_, _ = io.Copy(a.terminal, os.Stdin)
	}()

	<-stop
	return a.terminal.Close()
}
