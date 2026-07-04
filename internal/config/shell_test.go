package config_test

import (
	"testing"

	"github.com/termizard/termizard/internal/config"
)

func TestShellCommandNoOhMyZsh(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = true

	cmd := cfg.ShellCommand()
	if len(cmd) < 2 || cmd[0] != "/bin/zsh" || cmd[1] != "-f" {
		t.Fatalf("want [/bin/zsh -f ...], got %v", cmd)
	}
}

func TestShellCommandZshWithNoRCSFlag(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.Args = []string{"--no-rcs", "-l"}
	cfg.Shell.NoOhMyZsh = true

	cmd := cfg.ShellCommand()
	if cmd[0] != "/bin/zsh" || cmd[1] != "--no-rcs" || cmd[2] != "-l" {
		t.Fatalf("unexpected argv: %v", cmd)
	}
}

func TestShellCommandPreservesExplicitZshArgs(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "zsh"
	cfg.Shell.Args = []string{"-l"}
	cfg.Shell.NoOhMyZsh = true

	cmd := cfg.ShellCommand()
	if cmd[0] != "zsh" || cmd[1] != "-f" || cmd[2] != "-l" {
		t.Fatalf("unexpected argv: %v", cmd)
	}
}
