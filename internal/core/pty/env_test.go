package pty

import (
	"os"
	"path/filepath"
	"runtime"
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
	customDir := t.TempDir()
	customBin := filepath.Join(customDir, "custom", "bin")
	if err := os.MkdirAll(customBin, 0o755); err != nil {
		t.Fatal(err)
	}

	env := PrepareShellEnv([]string{"PATH=" + customBin, "HOME=" + customDir})
	path := envMap(env)["PATH"]
	if !strings.Contains(path, customBin) {
		t.Fatalf("PATH %q missing %q", path, customBin)
	}

	parts := strings.Split(path, string(os.PathListSeparator))
	if parts[0] != customBin {
		t.Fatalf("custom PATH entries should come first, got %q", path)
	}
	if len(parts) < 2 {
		t.Fatal("expected standard PATH entries to be merged")
	}

	merged := false
	for _, p := range parts[1:] {
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			merged = true
			break
		}
	}
	if !merged {
		t.Fatalf("expected at least one standard PATH directory, got %q", path)
	}
}

func TestClampSize(t *testing.T) {
	cols, rows := ClampSize(0, 0)
	if cols != 1 || rows != 1 {
		t.Fatalf("ClampSize(0,0) = (%d,%d), want (1,1)", cols, rows)
	}

	cols, rows = ClampSize(120, 40)
	if cols != 120 || rows != 40 {
		t.Fatalf("ClampSize(120,40) = (%d,%d), want (120,40)", cols, rows)
	}

	cols, rows = ClampSize(70000, 80000)
	if cols != 65535 || rows != 65535 {
		t.Fatalf("ClampSize overflow = (%d,%d), want (65535,65535)", cols, rows)
	}
}

func TestPrepareShellEnvNilUsesProcessEnv(t *testing.T) {
	t.Setenv("TERM", "vt100")
	env := PrepareShellEnv(nil)
	if envMap(env)["TERM"] != "vt100" {
		t.Fatalf("TERM = %q, want vt100", envMap(env)["TERM"])
	}
}

func TestSetIfEmptyPreservesExisting(t *testing.T) {
	m := map[string]string{"TERM": "screen"}
	setIfEmpty(m, "TERM", "xterm")
	if m["TERM"] != "screen" {
		t.Fatalf("TERM = %q, want screen", m["TERM"])
	}
}

func TestSetIfEmptySkipsBlankValue(t *testing.T) {
	m := map[string]string{}
	setIfEmpty(m, "TERM", "")
	if _, ok := m["TERM"]; ok {
		t.Fatal("empty value should not be set")
	}
}

func TestStandardPathEntriesNonEmpty(t *testing.T) {
	entries := standardPathEntries()
	if len(entries) == 0 {
		t.Fatal("expected standard path entries")
	}
}

func TestDefaultShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if sh := DefaultShell(); sh == "" {
		t.Fatal("DefaultShell returned empty")
	}
}

func TestPrepareShellEnvIgnoresGitBashSHELLOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	t.Setenv("SHELL", `C:\Program Files\Git\bin\bash.exe`)
	env := PrepareShellEnv([]string{"HOME=C:\\Users\\test"})
	shell := envMap(env)["SHELL"]
	if strings.Contains(strings.ToLower(shell), "bash") {
		t.Fatalf("SHELL = %q, want Windows shell not git bash", shell)
	}
}
