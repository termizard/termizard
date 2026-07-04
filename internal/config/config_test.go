package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/termizard/termizard/internal/config"
)

func TestEnsureDefaultFileCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "termizard", "config.toml")

	if err := config.EnsureDefaultFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Second call is a no-op.
	if err := config.EnsureDefaultFile(path); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Window.ShowTitleBar {
		t.Fatal("expected show_title_bar default true")
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.toml")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Terminal.InitialCols != config.Defaults().Terminal.InitialCols {
		t.Fatalf("InitialCols = %d, want default %d", cfg.Terminal.InitialCols, config.Defaults().Terminal.InitialCols)
	}
}

func TestLoadValidOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[window]\nshow_title_bar = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Window.ShowTitleBar {
		t.Fatal("expected show_title_bar false from file")
	}
}

func TestLoadInvalidTOMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[[window\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got := config.DefaultPath()
	want := filepath.Join(dir, "termizard", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestShellCommandUsesDefaultShell(t *testing.T) {
	t.Setenv("SHELL", "")
	cfg := config.Defaults()
	cfg.Shell.Program = ""

	cmd := cfg.ShellCommand()
	if len(cmd) == 0 || cmd[0] == "" {
		t.Fatalf("ShellCommand() = %v, want non-empty program", cmd)
	}
}

func TestShellCommandUsesEnvShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/fish")
	cfg := config.Defaults()
	cfg.Shell.Program = ""

	cmd := cfg.ShellCommand()
	if len(cmd) != 1 || cmd[0] != "/bin/fish" {
		t.Fatalf("ShellCommand() = %v, want [/bin/fish]", cmd)
	}
}

func TestEnsureDefaultFileParentIsFile(t *testing.T) {
	dir := t.TempDir()
	blocking := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocking, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocking, "termizard", "config.toml")
	if err := config.EnsureDefaultFile(path); err == nil {
		t.Fatal("expected error when parent path is a file")
	}
}

func TestDefaultPathFallbackHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := config.DefaultPath()
	want := filepath.Join(home, ".config", "termizard", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestShellCommandWithoutOhMyZshOnNonZshProgram(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/usr/bin/fish"
	cfg.Shell.Args = []string{"-l"}
	cfg.Shell.NoOhMyZsh = true

	cmd := cfg.ShellCommand()
	if len(cmd) != 2 || cmd[0] != "/usr/bin/fish" || cmd[1] != "-l" {
		t.Fatalf("ShellCommand() = %v, want [/usr/bin/fish -l]", cmd)
	}
}

func TestShellCommandNonZshSkipsNoOhMyZsh(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/bash"
	cfg.Shell.NoOhMyZsh = true

	cmd := cfg.ShellCommand()
	if len(cmd) != 1 || cmd[0] != "/bin/bash" {
		t.Fatalf("ShellCommand() = %v, want [/bin/bash]", cmd)
	}
}
