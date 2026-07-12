//go:build windows

package pty

import (
	"os"
	"path/filepath"
	"strings"
)

func platformDefaultShell() string {
	return windowsDefaultShell()
}

// windowsDefaultShell picks the best interactive shell on Windows:
// pwsh (PS 7+) → Windows PowerShell → COMSPEC → cmd.exe.
// Store / WindowsApps App Execution Aliases are skipped — they are 0-byte
// stubs that CreateProcess + ConPTY cannot spawn.
func windowsDefaultShell() string {
	for _, candidate := range windowsShellCandidates() {
		if isSpawnableExecutable(candidate) {
			return candidate
		}
	}
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	return filepath.Join(sysRoot, "System32", "cmd.exe")
}

func windowsShellCandidates() []string {
	var out []string
	add := func(paths ...string) {
		for _, p := range paths {
			if p != "" {
				out = append(out, p)
			}
		}
	}

	if pf := os.Getenv("ProgramFiles"); pf != "" {
		add(filepath.Join(pf, "PowerShell", "7", "pwsh.exe"))
		add(filepath.Join(pf, "PowerShell", "7-preview", "pwsh.exe"))
	}
	if pf86 := os.Getenv("ProgramFiles(x86)"); pf86 != "" {
		add(filepath.Join(pf86, "PowerShell", "7", "pwsh.exe"))
	}

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	// Inbox PowerShell (PS 5) before cmd.exe: users expect a PowerShell
	// experience. Fall back to cmd.exe only when PS 5 is not present.
	add(filepath.Join(sysRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
	add(filepath.Join(sysRoot, "System32", "cmd.exe"))

	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		add(comspec)
	}

	return out
}

func isSpawnableExecutable(path string) bool {
	if path == "" {
		return false
	}
	clean := strings.ToLower(filepath.Clean(path))
	if strings.Contains(clean, `\windowsapps\`) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	// App Execution Aliases are 0-byte reparse points.
	return info.Size() > 1024
}

func resolveSpawnablePath(path string) string {
	if path == "" {
		return ""
	}
	if isSpawnableExecutable(path) {
		return path
	}
	base := strings.ToLower(filepath.Base(path))
	switch {
	case base == "pwsh.exe" || base == "pwsh":
		for _, c := range windowsShellCandidates() {
			if isSpawnableExecutable(c) && strings.HasSuffix(strings.ToLower(c), `\pwsh.exe`) {
				return c
			}
		}
	case base == "powershell.exe" || base == "powershell":
		sysRoot := os.Getenv("SystemRoot")
		if sysRoot == "" {
			sysRoot = `C:\Windows`
		}
		ps5 := filepath.Join(sysRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if isSpawnableExecutable(ps5) {
			return ps5
		}
	case base == "cmd.exe" || base == "cmd":
		sysRoot := os.Getenv("SystemRoot")
		if sysRoot == "" {
			sysRoot = `C:\Windows`
		}
		cmd := filepath.Join(sysRoot, "System32", "cmd.exe")
		if isSpawnableExecutable(cmd) {
			return cmd
		}
	}
	return ""
}

func windowsDefaultCommand() []string {
	return ensureWindowsInteractive([]string{windowsDefaultShell()})
}

func normalizeWindowsCommand(cmd []string) []string {
	if len(cmd) == 0 || cmd[0] == "" {
		return windowsDefaultCommand()
	}
	if strings.HasPrefix(cmd[0], "/") {
		return windowsDefaultCommand()
	}
	out := ensureWindowsInteractive(cmd)
	if resolved := resolveSpawnablePath(out[0]); resolved != "" {
		out[0] = resolved
	}
	return out
}

func ensureWindowsInteractive(cmd []string) []string {
	if len(cmd) == 0 {
		return windowsDefaultCommand()
	}
	shell := strings.ToLower(strings.TrimSuffix(filepath.Base(cmd[0]), ".exe"))
	args := stripUnixShellArgs(cmd[1:])

	switch shell {
	case "pwsh", "powershell":
		if hasWindowsArg(args, "-Command", "-File", "-NoExit") {
			return append([]string{cmd[0]}, args...)
		}
		return append([]string{cmd[0], "-NoLogo", "-NoExit"}, args...)
	case "cmd":
		if hasWindowsArg(args, "/K", "/C", "/c") {
			return append([]string{cmd[0]}, args...)
		}
		return append([]string{cmd[0], "/K"}, args...)
	default:
		return append([]string{cmd[0]}, args...)
	}
}

func stripUnixShellArgs(args []string) []string {
	skip := map[string]struct{}{
		"-l": {}, "--login": {}, "-i": {}, "-f": {}, "--no-rcs": {},
		"--rcfile": {}, "--noprofile": {},
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if _, drop := skip[args[i]]; drop {
			if args[i] == "--rcfile" && i+1 < len(args) {
				i++
			}
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func hasWindowsArg(args []string, flags ...string) bool {
	set := make(map[string]struct{}, len(flags))
	for _, f := range flags {
		set[strings.ToLower(f)] = struct{}{}
	}
	for _, a := range args {
		if _, ok := set[strings.ToLower(a)]; ok {
			return true
		}
	}
	return false
}
