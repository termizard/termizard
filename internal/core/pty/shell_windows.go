//go:build windows

package pty

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func platformDefaultShell() string {
	return windowsDefaultShell()
}

// windowsDefaultShell picks the best interactive shell on Windows:
// pwsh (PS 7+) → Windows PowerShell → COMSPEC → cmd.exe.
func windowsDefaultShell() string {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}

	ps5 := filepath.Join(sysRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(ps5); err == nil {
		return ps5
	}

	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		if _, err := os.Stat(comspec); err == nil {
			return comspec
		}
	}

	cmd := filepath.Join(sysRoot, "System32", "cmd.exe")
	if _, err := os.Stat(cmd); err == nil {
		return cmd
	}
	return cmd
}

func normalizeWindowsCommand(cmd []string) []string {
	if len(cmd) == 0 || cmd[0] == "" {
		return []string{windowsDefaultShell()}
	}
	if strings.HasPrefix(cmd[0], "/") {
		out := []string{windowsDefaultShell()}
		return append(out, cmd[1:]...)
	}
	return cmd
}
