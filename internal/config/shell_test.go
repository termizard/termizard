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

func TestShellCommandBashBundledKaliPrompt(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/bash"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if len(cmd) < 4 || cmd[0] != "/bin/bash" || cmd[1] != "--rcfile" || cmd[3] != "--noprofile" {
		t.Fatalf("ShellCommand() = %v, want bundled bash rcfile", cmd)
	}
}

func TestShellEnvironmentBashBundledKaliSetsRC(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/bash"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	env, err := cfg.ShellEnvironment([]string{"HOME=/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERMIZARD_BASH_RC=") && strings.TrimPrefix(kv, "TERMIZARD_BASH_RC=") != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TERMIZARD_BASH_RC in env, got %v", env)
	}
}

func TestShellCommandBundledKaliNonZshShellUnchanged(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/usr/bin/fish"
	cfg.Shell.Args = []string{"-l"}
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if len(cmd) != 2 || cmd[0] != "/usr/bin/fish" || cmd[1] != "-l" {
		t.Fatalf("ShellCommand() = %v, want [/usr/bin/fish -l]", cmd)
	}
}

func TestShellCommandZshNoOhMyZshSkipsWhenFlagAlreadyPresent(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.Args = []string{"-f", "-l"}
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "none"

	cmd := cfg.ShellCommand()
	if len(cmd) != 3 || cmd[0] != "/bin/zsh" || cmd[1] != "-f" || cmd[2] != "-l" {
		t.Fatalf("ShellCommand() = %v, want existing -f preserved", cmd)
	}
}

func TestShellCommandZshRemovesFFlagForBundledKali(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.Args = []string{"-f", "-l"}
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if cmd[0] != "/bin/zsh" || cmd[1] != "-i" {
		t.Fatalf("ShellCommand() = %v, want -f stripped and -i added", cmd)
	}
	for _, arg := range cmd[1:] {
		if arg == "-f" {
			t.Fatalf("ShellCommand() = %v, must not keep -f for bundled kali", cmd)
		}
	}
}

func TestShellEnvironmentUsesEnvShellProgram(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	cfg := config.Defaults()
	cfg.Shell.Program = ""
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	env, err := cfg.ShellEnvironment([]string{"HOME=/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERMIZARD_BASH_RC=") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected bash bundled env when SHELL=/bin/bash, got %v", env)
	}
}

func TestShellEnvironmentNoBundledPromptPassthrough(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = false

	base := []string{"HOME=/tmp", "TERM=xterm-256color"}
	env, err := cfg.ShellEnvironment(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "ZDOTDIR=") {
			t.Fatalf("unexpected ZDOTDIR in env: %v", env)
		}
	}
}

func TestShellEnvironmentUsesDefaultShellProgram(t *testing.T) {
	t.Setenv("SHELL", "")
	cfg := config.Defaults()
	cfg.Shell.Program = ""
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	env, err := cfg.ShellEnvironment([]string{"HOME=/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(env) == 0 {
		t.Fatal("expected prepared env")
	}
}

func TestShellCommandWithoutOhMyZshEmptyProgramUsesDefaultShell(t *testing.T) {
	t.Setenv("SHELL", "")
	cfg := config.Defaults()
	cfg.Shell.Program = ""
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "none"

	cmd := cfg.ShellCommand()
	if len(cmd) < 2 || cmd[1] != "-f" {
		t.Fatalf("ShellCommand() = %v, want -f on default zsh shell", cmd)
	}
}

func TestShellCommandPrefixedZshBinary(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/opt/homebrew/bin/x86_64-linux-gnu-zsh"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	cmd := cfg.ShellCommand()
	if len(cmd) < 2 || cmd[1] != "-i" {
		t.Fatalf("ShellCommand() = %v, want interactive zsh for prefixed binary", cmd)
	}
}
