package app

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
)

type PTY interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Pid() int
	WaitContext(ctx context.Context) (uint32, error)
}

type App struct {
	terminal PTY
}

func NewApp(ptyInstance PTY) *App {
	return &App{terminal: ptyInstance}
}

func (a *App) Run() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Reading from PTY to console
	go func() {
		io.Copy(os.Stdout, a.terminal)
	}()

	// We write from the console to PTY
	go func() {
		io.Copy(a.terminal, os.Stdin)
	}()

	<-stop
	return a.terminal.Close()
}
