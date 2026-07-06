//go:build windows

package pty

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsDefaultShellNotUnixPath(t *testing.T) {
	t.Setenv("SHELL", "")
	sh := windowsDefaultShell()
	if sh == "" {
		t.Fatal("windowsDefaultShell returned empty")
	}
	if strings.HasPrefix(sh, "/") {
		t.Fatalf("windowsDefaultShell = %q, want Windows path", sh)
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
	if len(got) != 2 || got[1] != "-l" {
		t.Fatalf("normalizeWindowsCommand = %v, want Windows shell with -l arg", got)
	}
}

func TestResolveCommandEmptyUsesWindowsShell(t *testing.T) {
	got, err := resolveCommand(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] == "" {
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
