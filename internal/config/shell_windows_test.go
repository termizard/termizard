//go:build windows

package config_test

import (
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/config"
)

func TestShellCommandIgnoresGitBashShellEnv(t *testing.T) {
	t.Setenv("SHELL", `C:\Program Files\Git\bin\bash.exe`)
	cfg := config.Defaults()
	cfg.Shell.Program = ""

	cmd := cfg.ShellCommand()
	if len(cmd) == 0 {
		t.Fatal("ShellCommand() empty")
	}
	if strings.Contains(strings.ToLower(cmd[0]), "bash") {
		t.Fatalf("ShellCommand() = %v, must not use Git Bash on Windows", cmd)
	}
}
