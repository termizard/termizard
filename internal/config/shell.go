package config

import (
	"os"
	"path/filepath"

	"github.com/termizard/termizard/internal/core/pty"
)

// ShellCommand returns the argv for the PTY child process.
func (cfg *Config) ShellCommand() []string {
	var cmd []string
	if cfg.Shell.Program != "" {
		cmd = append([]string{cfg.Shell.Program}, cfg.Shell.Args...)
	} else if sh := os.Getenv("SHELL"); sh != "" {
		cmd = []string{sh}
	} else {
		cmd = []string{pty.DefaultShell()}
	}
	if cfg.Shell.NoOhMyZsh {
		cmd = withoutOhMyZsh(cmd)
	}
	return cmd
}

// withoutOhMyZsh starts zsh without reading .zshrc/.zshenv (skips oh-my-zsh).
func withoutOhMyZsh(cmd []string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	if filepath.Base(cmd[0]) != "zsh" {
		return cmd
	}
	for _, arg := range cmd[1:] {
		if arg == "-f" || arg == "--no-rcs" {
			return cmd
		}
	}
	args := append([]string{"-f"}, cmd[1:]...)
	return append([]string{cmd[0]}, args...)
}
