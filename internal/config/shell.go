package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/termizard/termizard/internal/core/pty"
	"github.com/termizard/termizard/internal/shell/prompt"
)

// ShellCommand returns the argv for the PTY child process.
func (cfg *Config) ShellCommand() []string {
	cmd := cfg.defaultShellArgv()
	if cfg.usesBundledPrompt() {
		_, bashRC, err := prompt.Materialize()
		psProfile, _ := prompt.PowerShellProfilePath()
		if err == nil {
			return withBundledShell(cmd, bashRC, psProfile)
		}
	}
	if cfg.Shell.NoOhMyZsh {
		cmd = withoutOhMyZsh(cmd)
	}
	// On Windows, default PowerShell does not emit OSC cwd titles. Inject a
	// minimal title.ps1 so tabs/window match the path shown in `PS C:\...>`.
	if runtime.GOOS == goosWindows {
		cmd = withPowerShellCwdTitle(cmd)
	}
	return cmd
}

func (cfg *Config) defaultShellArgv() []string {
	if cfg.Shell.Program != "" {
		return append([]string{cfg.Shell.Program}, cfg.Shell.Args...)
	}
	// Git for Windows sets $SHELL to bash.exe; ConPTY cannot run it reliably.
	if runtime.GOOS != goosWindows {
		if sh := os.Getenv("SHELL"); sh != "" {
			return []string{sh}
		}
	}
	return []string{pty.DefaultShell()}
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
	if runtime.GOOS != goosWindows {
		if sh := os.Getenv("SHELL"); sh != "" {
			return sh
		}
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
	if cfg.promptStyle() != prompt.StyleKali {
		return false
	}
	if cfg.Shell.NoOhMyZsh {
		return true
	}
	return false
}

const (
	shellZsh        = "zsh"
	shellPowerShell = "powershell"
	shellPwsh       = "pwsh"
	psNoLogo        = "-NoLogo"
	psNoExit        = "-NoExit"
	psFile          = "-File"
)

func withBundledShell(cmd []string, bashRC, psProfile string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	switch shellBase(cmd[0]) {
	case shellZsh:
		return withInteractiveZsh(cmd)
	case "bash":
		return withBundledBash(cmd, bashRC)
	case shellPwsh, shellPowerShell:
		return withBundledPowerShell(cmd, psProfile)
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

func withBundledPowerShell(cmd []string, psProfile string) []string {
	if psProfile != "" {
		return []string{
			cmd[0], psNoLogo, psNoExit,
			"-ExecutionPolicy", "Bypass",
			psFile, psProfile,
		}
	}
	return append([]string{cmd[0], psNoLogo, psNoExit}, cmd[1:]...)
}

// withPowerShellCwdTitle injects title.ps1 for pwsh/powershell so OSC 0/2
// carries $PWD (ignored for non-PowerShell argv).
func withPowerShellCwdTitle(cmd []string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	switch shellBase(cmd[0]) {
	case shellPwsh, shellPowerShell:
	default:
		return cmd
	}
	// Already using an explicit -File / -Command (e.g. kali profile).
	if hasArg(cmd[1:], psFile) || hasArg(cmd[1:], "-Command") || hasArg(cmd[1:], "-c") {
		return cmd
	}
	titlePS1, err := prompt.PowerShellTitlePath()
	if err != nil || titlePS1 == "" {
		return cmd
	}
	return []string{
		cmd[0], psNoLogo, psNoExit,
		"-ExecutionPolicy", "Bypass",
		psFile, titlePS1,
	}
}

func withoutOhMyZsh(cmd []string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	if shellBase(cmd[0]) != shellZsh {
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
	base := filepath.Base(strings.ReplaceAll(shellPath, `\`, `/`))
	if i := strings.LastIndex(base, "-"); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
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
