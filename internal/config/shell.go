package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/shell/prompt"
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
	if cfg.usesBundledPrompt() {
		_, bashRC, err := prompt.Materialize()
		if err == nil {
			cmd = withBundledShell(cmd, bashRC)
			return cmd
		}
	}
	if cfg.Shell.NoOhMyZsh {
		cmd = withoutOhMyZsh(cmd)
	}
	return cmd
}

// ShellEnvironment returns env vars for the PTY child, including bundled prompt setup.
func (cfg *Config) ShellEnvironment(base []string) ([]string, error) {
	env := pty.PrepareShellEnv(base)
	if !cfg.usesBundledPrompt() {
		return env, nil
	}
	return prompt.ApplyEnv(env, cfg.shellProgram(), cfg.promptStyle())
}

func (cfg *Config) shellProgram() string {
	if cfg.Shell.Program != "" {
		return cfg.Shell.Program
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return pty.DefaultShell()
}

func (cfg *Config) promptStyle() string {
	if cfg.Shell.Prompt == "" {
		return prompt.StyleKali
	}
	return cfg.Shell.Prompt
}

func (cfg *Config) usesBundledPrompt() bool {
	return cfg.Shell.NoOhMyZsh && cfg.promptStyle() == prompt.StyleKali
}

func withBundledShell(cmd []string, bashRC string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	switch shellBase(cmd[0]) {
	case "zsh":
		return withInteractiveZsh(cmd)
	case "bash":
		return withBundledBash(cmd, bashRC)
	default:
		return withoutOhMyZsh(cmd)
	}
}

func withInteractiveZsh(cmd []string) []string {
	args := cmd[1:]
	if hasArg(args, "-f") || hasArg(args, "--no-rcs") {
		args = removeArgs(args, "-f", "--no-rcs")
	}
	if !hasArg(args, "-i") {
		args = append([]string{"-i"}, args...)
	}
	return append([]string{cmd[0]}, args...)
}

func withBundledBash(cmd []string, rcPath string) []string {
	args := []string{"--rcfile", rcPath, "--noprofile", "-i"}
	return append([]string{cmd[0]}, args...)
}

func withoutOhMyZsh(cmd []string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	if shellBase(cmd[0]) != "zsh" {
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

func shellBase(shellPath string) string {
	base := filepath.Base(shellPath)
	if i := strings.LastIndex(base, "-"); i >= 0 {
		base = base[i+1:]
	}
	return base
}

func hasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func removeArgs(args []string, flags ...string) []string {
	skip := make(map[string]struct{}, len(flags))
	for _, f := range flags {
		skip[f] = struct{}{}
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		if _, ok := skip[a]; ok {
			continue
		}
		out = append(out, a)
	}
	return out
}
