//go:build darwin || linux

package pty

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"

	creackpty "github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/termizard/termizard/internal/util/logger"
)

// unixPTY implements PTY on Linux, macOS, and BSD systems.
// Under the hood it uses github.com/creack/pty which wraps openpty(3)
// and handles TIOCSWINSZ ioctl for resize.
type unixPTY struct {
	master *os.File  // the PTY master fd — we read/write here
	cmd    *exec.Cmd // the child shell process
}

func Open(cfg Config) (PTY, error) {
	command, err := resolveCommand(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("pty: resolve command: %w", err)
	}
	dir, err := resolveDir(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("pty: resolve dir: %w", err)
	}
	env := PrepareShellEnv(cfg.Env)

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	cmd.Dir = dir

	// StartWithSize creates a PTY pair, forks the child into a new session,
	// sets the controlling terminal, and calls execve — all in one call chain.
	master, err := creackpty.StartWithSize(cmd, &creackpty.Winsize{
		Rows: cfg.Rows,
		Cols: cfg.Cols,
	})
	if err != nil {
		return nil, fmt.Errorf("pty: start %q: %w", command[0], err)
	}

	logger.Get().Info("pty opened",
		slog.String("cmd", command[0]),
		slog.Int("pid", cmd.Process.Pid),
		slog.Uint64("cols", uint64(cfg.Cols)),
		slog.Uint64("rows", uint64(cfg.Rows)),
	)

	return &unixPTY{master: master, cmd: cmd}, nil
}

func (p *unixPTY) Read(buf []byte) (int, error) {
	return p.master.Read(buf)
}

func (p *unixPTY) Write(buf []byte) (int, error) {
	return p.master.Write(buf)
}

// Resize informs the kernel of new terminal dimensions.
// TIOCSWINSZ delivers SIGWINCH to the child's process group automatically.
func (p *unixPTY) Resize(cols, rows uint16) error {
	err := unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Col: cols,
		Row: rows,
	})
	if err != nil {
		logger.Get().Warn("pty resize failed",
			slog.Int("pid", p.cmd.Process.Pid),
			slog.String("err", err.Error()),
		)
	} else {
		logger.Get().Debug("pty resized",
			slog.Int("pid", p.cmd.Process.Pid),
			slog.Uint64("cols", uint64(cols)),
			slog.Uint64("rows", uint64(rows)),
		)
	}
	return err
}

func (p *unixPTY) Fd() uintptr { return p.master.Fd() }
func (p *unixPTY) Pid() int    { return p.cmd.Process.Pid }

func (p *unixPTY) Wait() error {
	err := p.cmd.Wait()
	logger.Get().Info("pty child exited", slog.Int("pid", p.cmd.Process.Pid))
	return err
}

func (p *unixPTY) Close() error {
	if p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			logger.Get().Warn("pty kill failed",
				slog.Int("pid", p.cmd.Process.Pid),
				slog.String("err", err.Error()),
			)
		}
	}
	logger.Get().Info("pty closed", slog.Int("pid", p.cmd.Process.Pid))
	return p.master.Close()
}

// --- Internal helpers ---

func resolveCommand(cmd []string) ([]string, error) {
	if len(cmd) > 0 {
		return cmd, nil
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return []string{shell}, nil
	}
	return []string{"/bin/sh"}, nil
}

func resolveDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("pty: cannot determine home dir: %w", err)
	}
	return u.HomeDir, nil
}
