//go:build windows

package pty

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSpawnableExecutableRejectsWindowsApps(t *testing.T) {
	if isSpawnableExecutable(`C:\Users\me\AppData\Local\Microsoft\WindowsApps\pwsh.exe`) {
		t.Fatal("WindowsApps alias should not be spawnable")
	}
}

func TestResolveSpawnablePathSkipsStub(t *testing.T) {
	got := resolveSpawnablePath(`C:\Users\me\AppData\Local\Microsoft\WindowsApps\pwsh.exe`)
	if got != "" && strings.Contains(strings.ToLower(got), `\windowsapps\`) {
		t.Fatalf("resolveSpawnablePath = %q, want real install path", got)
	}
}

func TestNormalizeWindowsCommandReplacesUnixShell(t *testing.T) {
	got := normalizeWindowsCommand([]string{"/bin/sh", "-l"})
	if len(got) == 0 {
		t.Fatal("normalizeWindowsCommand returned empty")
	}
	if strings.HasPrefix(got[0], "/") {
		t.Fatalf("command[0] = %q, want Windows shell", got[0])
	}
	for _, arg := range got[1:] {
		if arg == "-l" {
			t.Fatalf("normalizeWindowsCommand = %v, unix -l should be stripped", got)
		}
	}
	if strings.Contains(strings.ToLower(got[0]), `\windowsapps\`) {
		t.Fatalf("normalizeWindowsCommand = %v, must not use WindowsApps stub", got)
	}
}

func TestNormalizeWindowsCommandInteractivePwsh(t *testing.T) {
	pf := filepath.Join(`C:\Program Files`, "PowerShell", "7", "pwsh.exe")
	got := normalizeWindowsCommand([]string{pf})
	if !hasWindowsArg(got[1:], "-NoExit") {
		t.Fatalf("normalizeWindowsCommand = %v, want -NoExit for interactive pwsh", got)
	}
}

func TestResolveCommandEmptyUsesWindowsShell(t *testing.T) {
	got, err := resolveCommand(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0] == "" {
		t.Fatalf("resolveCommand(nil) = %v", got)
	}
	if strings.HasPrefix(got[0], "/") {
		t.Fatalf("resolveCommand(nil) = %v, want Windows path", got)
	}
}

func TestResolveCommandPreservesExplicitProgram(t *testing.T) {
	want := filepath.Join(`C:\Windows`, "System32", "cmd.exe")
	got, err := resolveCommand([]string{want, "/c", "echo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != want {
		t.Fatalf("resolveCommand = %v, want %q prefix", got, want)
	}
}
