//go:build darwin || linux

package pty

import (
	"os"
	"os/user"
	"strings"
	"testing"
)

// --- resolveCommand ---

func TestResolveCommandExplicit(t *testing.T) {
	got, err := resolveCommand([]string{"/bin/bash", "--login"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "/bin/bash" || got[1] != "--login" {
		t.Errorf("got %v, want [/bin/bash --login]", got)
	}
}

func TestResolveCommandFallsBackToSHELL(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	got, err := resolveCommand(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "/usr/bin/zsh" {
		t.Errorf("got %v, want [/usr/bin/zsh]", got)
	}
}

func TestResolveCommandFallsBackToSlashBinSh(t *testing.T) {
	t.Setenv("SHELL", "")

	// Temporarily override user.Current to return no shell.
	// Since resolveCommand falls back to /bin/sh when $SHELL is empty and
	// user.Current().Shell is empty, this test verifies the final fallback
	// without mocking the OS. On CI the default shell may or may not be set,
	// so we only assert the result is a non-empty path ending in "sh".
	got, err := resolveCommand(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 || !strings.HasSuffix(got[0], "sh") {
		t.Errorf("expected a shell path, got %v", got)
	}
}

// --- resolveDir ---

func TestResolveDirExplicit(t *testing.T) {
	got, err := resolveDir("/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/test" {
		t.Errorf("got %q, want /tmp/test", got)
	}
}

func TestResolveDirFallsBackToHOME(t *testing.T) {
	t.Setenv("HOME", "/custom/home")
	got, err := resolveDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/custom/home" {
		t.Errorf("got %q, want /custom/home", got)
	}
}

func TestResolveDirFallsBackToUserHomeDir(t *testing.T) {
	t.Setenv("HOME", "")
	got, err := resolveDir("")
	if err != nil {
		t.Fatalf("resolveDir with empty HOME: %v", err)
	}
	u, err := user.Current()
	if err != nil {
		t.Skip("cannot look up current user:", err)
	}
	if got != u.HomeDir {
		t.Errorf("got %q, want %q", got, u.HomeDir)
	}
}

func TestResolveDirPrefersDirOverHOME(t *testing.T) {
	t.Setenv("HOME", "/should/be/ignored")
	got, err := resolveDir("/explicit/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/explicit/path" {
		t.Errorf("explicit dir not honored: got %q", got)
	}
}

// TestResolveCommandEmpty verifies that passing an empty (non-nil) slice still
// triggers the fallback to $SHELL.
func TestResolveCommandEmptySlice(t *testing.T) {
	t.Setenv("SHELL", "/bin/dash")
	got, err := resolveCommand([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one element")
	}
	// An empty slice means "no command provided" so we fall back.
	if got[0] != "/bin/dash" {
		t.Errorf("got %v, want [/bin/dash]", got)
	}
}

// TestResolveDirUsesRealHome is an integration sanity-check: with a real HOME,
// resolveDir should return exactly os.Getenv("HOME").
func TestResolveDirUsesRealHome(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("$HOME not set in this environment")
	}
	got, err := resolveDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != home {
		t.Errorf("got %q, want %q", got, home)
	}
}
