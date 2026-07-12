package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/config"
)

func TestTabItemWorkingDirEmptyUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	item := config.TabItem{}
	if got := item.WorkingDir(); got != home {
		t.Fatalf("WorkingDir() = %q, want %q", got, home)
	}
}

func TestTabItemWorkingDirTildeUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	item := config.TabItem{Cwd: "~"}
	if got := item.WorkingDir(); got != home {
		t.Fatalf("WorkingDir() = %q, want %q", got, home)
	}
}

func TestTabItemWorkingDirTildeSlashJoinsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	item := config.TabItem{Cwd: "~/Projects/termizard"}
	want := filepath.Join(home, "Projects", "termizard")
	if got := item.WorkingDir(); got != want {
		t.Fatalf("WorkingDir() = %q, want %q", got, want)
	}
}

func TestTabItemWorkingDirAbsolutePath(t *testing.T) {
	item := config.TabItem{Cwd: "/var/tmp/work"}
	if got := item.WorkingDir(); got != "/var/tmp/work" {
		t.Fatalf("WorkingDir() = %q, want absolute path", got)
	}
}

func TestTabItemWorkingDirUsesUserHomeDirWhenHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	want, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}

	item := config.TabItem{Cwd: "  "}
	if got := item.WorkingDir(); got != want {
		t.Fatalf("WorkingDir() = %q, want %q", got, want)
	}
}

func TestTabItemWorkingDirTildeSlashUsesUserHomeDir(t *testing.T) {
	t.Setenv("HOME", "")
	want, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}

	item := config.TabItem{Cwd: "~/src"}
	got := item.WorkingDir()
	if got != filepath.Join(want, "src") {
		t.Fatalf("WorkingDir() = %q, want %q", got, filepath.Join(want, "src"))
	}
}

func TestTabItemShellCommandArgsOnlyUsesOverridePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("$SHELL is ignored on Windows; native shell detection is used instead")
	}
	cfg := config.Defaults()
	t.Setenv("SHELL", "/bin/zsh")

	item := config.TabItem{Args: []string{"-l"}}
	cmd := item.ShellCommand(cfg)
	// Args-only override replaces shell config; empty program falls back to $SHELL.
	if len(cmd) != 1 || cmd[0] != "/bin/zsh" {
		t.Fatalf("ShellCommand() = %v, want [/bin/zsh]", cmd)
	}
}

func TestTabItemShellCommandInheritsBundledKali(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "kali"

	item := config.TabItem{Program: "/bin/zsh"}
	cmd := item.ShellCommand(cfg)
	if len(cmd) < 2 || cmd[1] != "-i" {
		t.Fatalf("ShellCommand() = %v, want interactive zsh for kali", cmd)
	}
}

func TestTabItemDisplayTitleWhitespaceFallback(t *testing.T) {
	item := config.TabItem{Title: "   "}
	if got := item.DisplayTitle(2); got != "Tab 3" {
		t.Fatalf("DisplayTitle(2) = %q, want Tab 3", got)
	}
}

func TestTabItemWorkingDirTrimsSpaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	item := config.TabItem{Cwd: "  ~  "}
	if got := item.WorkingDir(); got != home {
		t.Fatalf("WorkingDir() = %q, want trimmed home %q", got, home)
	}
}

func TestTabItemWorkingDirRelativePath(t *testing.T) {
	item := config.TabItem{Cwd: "Projects/termizard"}
	if got := item.WorkingDir(); got != "Projects/termizard" {
		t.Fatalf("WorkingDir() = %q, want relative path unchanged", got)
	}
}

func TestTabItemWorkingDirTildeSlashUsesHomeEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	item := config.TabItem{Cwd: "~/Downloads"}
	want := filepath.Join(home, "Downloads")
	if got := item.WorkingDir(); got != want {
		t.Fatalf("WorkingDir() = %q, want %q", got, want)
	}
}

func TestTabItemShellCommandPreservesGlobalNoOhMyZsh(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "none"

	item := config.TabItem{Program: "/bin/zsh"}
	cmd := item.ShellCommand(cfg)
	if len(cmd) < 2 || cmd[1] != "-f" {
		t.Fatalf("ShellCommand() = %v, want -f for bare zsh", cmd)
	}
	if strings.Contains(strings.Join(cmd, " "), "-i") {
		t.Fatalf("ShellCommand() = %v, must not be interactive", cmd)
	}
}
