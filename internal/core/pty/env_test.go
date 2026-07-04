package pty

import (
	"strings"
	"testing"
)

func TestPrepareShellEnvSetsTermForGUI(t *testing.T) {
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")

	env := PrepareShellEnv([]string{"HOME=/tmp/testhome"})
	m := envMap(env)

	if got := m["TERM"]; got != "xterm-256color" {
		t.Fatalf("TERM = %q, want xterm-256color", got)
	}
	if got := m["COLORTERM"]; got != "truecolor" {
		t.Fatalf("COLORTERM = %q, want truecolor", got)
	}
	if got := m["LANG"]; got != "en_US.UTF-8" {
		t.Fatalf("LANG = %q, want en_US.UTF-8", got)
	}
	if got := m["SHELL"]; got == "" {
		t.Fatal("SHELL should be set")
	}
}

func TestPrepareShellEnvPreservesExistingTerm(t *testing.T) {
	env := PrepareShellEnv([]string{"TERM=screen-256color", "HOME=/x"})
	m := envMap(env)
	if m["TERM"] != "screen-256color" {
		t.Fatalf("TERM = %q, want screen-256color", m["TERM"])
	}
}

func TestPrepareShellEnvMergesPATH(t *testing.T) {
	env := PrepareShellEnv([]string{"PATH=/custom/bin", "HOME=/x"})
	path := envMap(env)["PATH"]
	if !strings.Contains(path, "/custom/bin") {
		t.Fatalf("PATH %q missing /custom/bin", path)
	}
	if !strings.Contains(path, "/usr/bin") {
		t.Fatalf("PATH %q missing /usr/bin", path)
	}
	if strings.Index(path, "/custom/bin") > strings.Index(path, "/usr/bin") {
		t.Fatalf("custom PATH entries should come first, got %q", path)
	}
}

func TestDefaultShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if sh := DefaultShell(); sh == "" {
		t.Fatal("DefaultShell returned empty")
	}
}
