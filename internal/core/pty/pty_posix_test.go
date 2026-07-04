//go:build darwin || linux

package pty

import (
	"os"
	"os/user"
	"strings"
	"testing"
)

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
	got, err := resolveCommand(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 || !strings.HasSuffix(got[0], "sh") {
		t.Errorf("expected a shell path, got %v", got)
	}
}

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

func TestResolveCommandEmptySlice(t *testing.T) {
	t.Setenv("SHELL", "/bin/dash")
	got, err := resolveCommand([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one element")
	}
	if got[0] != "/bin/dash" {
		t.Errorf("got %v, want [/bin/dash]", got)
	}
}

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
