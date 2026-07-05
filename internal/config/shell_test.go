package config_test

import (
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/config"
)

func TestShellCommandNoOhMyZshKaliPrompt(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if len(cmd) < 2 || cmd[0] != "/bin/zsh" || cmd[1] != "-i" {
		t.Fatalf("want [/bin/zsh -i ...], got %v", cmd)
	}
	for _, arg := range cmd[1:] {
		if arg == "-f" || arg == "--no-rcs" {
			t.Fatalf("bundled kali prompt must not use -f/--no-rcs, got %v", cmd)
		}
	}
}

func TestShellCommandNoOhMyZshBare(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "none"

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
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if cmd[0] != "/bin/zsh" || cmd[1] != "-i" {
		t.Fatalf("unexpected argv: %v", cmd)
	}
}

func TestShellCommandPreservesExplicitZshArgs(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "zsh"
	cfg.Shell.Args = []string{"-l"}
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if cmd[0] != "zsh" || cmd[1] != "-i" || cmd[2] != "-l" {
		t.Fatalf("unexpected argv: %v", cmd)
	}
}

func TestShellEnvironmentSetsZdotdirForKali(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	env, err := cfg.ShellEnvironment([]string{"HOME=/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "ZDOTDIR=") && strings.TrimPrefix(kv, "ZDOTDIR=") != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ZDOTDIR in env, got %v", env)
	}
}

func TestShellWithOhMyZshDoesNotInjectPrompt(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = false
	cfg.Shell.Prompt = "kali"

	env, err := cfg.ShellEnvironment([]string{"HOME=/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "ZDOTDIR=") {
			t.Fatalf("must not set ZDOTDIR when no_oh_my_zsh=false, got %v", env)
		}
	}

	cmd := cfg.ShellCommand()
	if len(cmd) != 1 || cmd[0] != "/bin/zsh" {
		t.Fatalf("ShellCommand() = %v, want plain [/bin/zsh]", cmd)
	}
}
