//go:build windows

package pty

import (
	"context"
	"fmt"
	"os"

	"github.com/admpub/conpty"
)

// WinPTY implements a PTY interface on top of ConPTY.
type WinPTY struct {
	c *conpty.ConPty
}

// StartPTY starts a command in ConPTY and returns a wrapper.
func StartPTY(command string, args ...string) (*WinPTY, error) {
	cmdWithArgs := append([]string{command}, args...)
	c, err := conpty.Start(cmdWithArgs)
	if err != nil {
		return nil, err
	}
	return &WinPTY{c: c}, nil
}

// Read/Write/Close/Resize/Pid are implemented via ConPty methods.
func (p *WinPTY) Read(b []byte) (int, error)  { return p.c.Read(b) }
func (p *WinPTY) Write(b []byte) (int, error) { return p.c.Write(b) }
func (p *WinPTY) Close() error                { return p.c.Close() }
func (p *WinPTY) Resize(cols, rows int) error { return p.c.Resize(cols, rows) }
func (p *WinPTY) Pid() int {
	if p.c == nil {
		return 0
	}
	return p.c.Pid()
}

// WaitContext is a wrapper for waiting for a process to terminate.
func (p *WinPTY) WaitContext(ctx context.Context) (uint32, error) {
	if p.c == nil {
		return 0, fmt.Errorf("no conpty")
	}
	return p.c.Wait(ctx)
}

// GetDefaultShell returns the path to the shell on Windows.
func GetDefaultShell() string {
	if shell := os.Getenv("COMSPEC"); shell != "" {
		return shell
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "powershell.exe"
}
