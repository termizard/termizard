//go:build linux || darwin

package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// UnixPTY implements the PTY interface for Unix systems (Linux/macOS).
type UnixPTY struct {
	f   *os.File
	cmd *exec.Cmd
}

// StartPTY starts the specified command in a new PTY session.
func StartPTY(command string, args ...string) (PTY, error) {
	c := exec.Command(command, args...)
	// Set the UTF-8 environment so that the shell correctly outputs Emoji
	c.Env = os.Environ()
	c.Env = append(c.Env, "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8", "TERM=xterm-256color")

	f, err := pty.Start(c)
	if err != nil {
		return nil, err
	}
	return &UnixPTY{f: f, cmd: c}, nil
}

func (p *UnixPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *UnixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *UnixPTY) Close() error                { return p.f.Close() }
func (p *UnixPTY) Resize(cols, rows int) error {
	return pty.Setsize(p.f, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
func (p *UnixPTY) Pid() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// WaitContext waits for the child process to terminate or the context to be canceled.
// Returns the process exit code (uint32) and the error (if any).
func (p *UnixPTY) WaitContext(ctx context.Context) (uint32, error) {
	if p.cmd == nil {
		return 0, fmt.Errorf("no command associated with PTY")
	}

	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case err := <-done:
		// the process completed without error
		if err == nil {
			if p.cmd.ProcessState != nil {
				if ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
					return uint32(ws.ExitStatus()), nil
				}
			}
			return 0, nil
		}

		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if ws, wsOk := exitErr.Sys().(syscall.WaitStatus); wsOk {
				return uint32(ws.ExitStatus()), nil
			}
		}

		return 0, err
	}
}

// GetDefaultShell returns the path to the shell on Unix.
func GetDefaultShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}
