package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "[colors]") || strings.Contains(content, "[[keybindings]]") {
		t.Fatalf("EnsureDefaultFile wrote full config, want minimal template:\n%s", content)
	}
	if !strings.Contains(content, "[window]") {
		t.Fatalf("expected [window] section in minimal config:\n%s", content)
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
	if runtime.GOOS == "windows" {
		t.Skip("$SHELL is ignored on Windows; native shell detection is used instead")
	}
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
	t.Setenv("USERPROFILE", home)

	got := config.DefaultPath()
	want := filepath.Join(home, ".config", "termizard", "config.toml")
	if got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathUsesUserProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)

	got := config.DefaultPath()
	want := filepath.Join(dir, ".config", "termizard", "config.toml")
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

func TestLoadNoOhMyZshFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[shell]\nno_oh_my_zsh = true\nprogram = \"/bin/zsh\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Shell.NoOhMyZsh {
		t.Fatal("expected no_oh_my_zsh true from file")
	}
	cmd := cfg.ShellCommand()
	if len(cmd) < 2 || cmd[1] != "-i" {
		t.Fatalf("ShellCommand() = %v, want -i for bundled kali prompt", cmd)
	}
	env, err := cfg.ShellEnvironment(nil)
	if err != nil {
		t.Fatal(err)
	}
	hasZdot := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "ZDOTDIR=") {
			hasZdot = true
		}
	}
	if !hasZdot {
		t.Fatalf("expected ZDOTDIR in env, got %v", env)
	}
}

func TestShellCommandNonZshSkipsNoOhMyZsh(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/bash"
	cfg.Shell.NoOhMyZsh = true
	cfg.Shell.Prompt = "none"

	cmd := cfg.ShellCommand()
	if len(cmd) != 1 || cmd[0] != "/bin/bash" {
		t.Fatalf("ShellCommand() = %v, want [/bin/bash]", cmd)
	}
}

func TestTitlePositionDefault(t *testing.T) {
	cfg := config.Defaults()
	if cfg.Window.TitlePosition != config.TitleAlignCenter {
		t.Fatalf("TitlePosition = %q, want center", cfg.Window.TitlePosition)
	}
}

func TestLoadTitlePosition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[window]\ntitle_position = \"left\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Window.TitlePosition != config.TitleAlignLeft {
		t.Fatalf("TitlePosition = %q, want left", cfg.Window.TitlePosition)
	}
}

func TestLoadInvalidTitlePositionFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[window]\ntitle_position = \"diagonal\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Window.TitlePosition != config.TitleAlignCenter {
		t.Fatalf("TitlePosition = %q, want center fallback", cfg.Window.TitlePosition)
	}
}

func TestLoadTabsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	toml := `
[tabs]
enabled = true
label = "index"
size = "full"
show_new_button = false
show_when_single = true
`
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Tabs.Enabled {
		t.Fatal("expected tabs enabled")
	}
	if cfg.Tabs.Label != config.TabLabelIndex {
		t.Fatalf("label = %q, want index", cfg.Tabs.Label)
	}
	if cfg.Tabs.Size != config.TabSizeFull {
		t.Fatalf("size = %q, want full", cfg.Tabs.Size)
	}
	if cfg.Tabs.ShowNewButton {
		t.Fatal("expected show_new_button false")
	}
	if !cfg.Tabs.ShowWhenSingle {
		t.Fatal("expected show_when_single true")
	}
}

func TestTabItemShellCommandOverride(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.Prompt = "none"
	item := config.TabItem{Program: "/bin/bash", Args: []string{"-l"}}
	cmd := item.ShellCommand(cfg)
	if len(cmd) != 2 || cmd[0] != "/bin/bash" {
		t.Fatalf("ShellCommand() = %v, want [/bin/bash -l]", cmd)
	}
}

func TestTabItemShellCommandFallback(t *testing.T) {
	cfg := config.Defaults()
	cfg.Shell.Program = "/bin/zsh"
	cfg.Shell.Prompt = "none"
	item := config.TabItem{Title: "main"}
	cmd := item.ShellCommand(cfg)
	if len(cmd) != 1 || cmd[0] != "/bin/zsh" {
		t.Fatalf("ShellCommand() = %v, want [/bin/zsh]", cmd)
	}
}

func TestTabItemDisplayTitle(t *testing.T) {
	item := config.TabItem{}
	if item.DisplayTitle(0) != "Tab 1" {
		t.Fatalf("DisplayTitle(0) = %q", item.DisplayTitle(0))
	}
	item.Title = "shell"
	if item.DisplayTitle(1) != "shell" {
		t.Fatalf("DisplayTitle(1) = %q", item.DisplayTitle(1))
	}
}

func TestTabsActive(t *testing.T) {
	cfg := config.Defaults()
	if !cfg.TabsActive() {
		t.Fatal("expected tabs active by default")
	}
	cfg.Tabs.Enabled = false
	if cfg.TabsActive() {
		t.Fatal("expected tabs inactive when disabled")
	}
}

func TestLoadInvalidTabLabelFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[tabs]\nlabel = \"bogus\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tabs.Label != config.TabLabelPath {
		t.Fatalf("label = %q, want path fallback", cfg.Tabs.Label)
	}
}

func TestLoadDirectoryPathReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected error when config path is a directory")
	}
}

func TestDefaultPathUsesUserHomeDirFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	want, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}

	got := config.DefaultPath()
	expect := filepath.Join(want, ".config", "termizard", "config.toml")
	if got != expect {
		t.Fatalf("DefaultPath() = %q, want %q", got, expect)
	}
}

func TestEnsureDefaultFileStatPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission test not supported on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("chmod-based permission test unreliable as root")
	}
	dir := t.TempDir()
	parent := filepath.Join(dir, "blocked")
	if err := os.Mkdir(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	path := filepath.Join(parent, "termizard", "config.toml")
	if err := config.EnsureDefaultFile(path); err == nil {
		t.Fatal("expected error when parent directory is not accessible")
	}
}

func TestEnsureDefaultFileStatNotDirectory(t *testing.T) {
	path := filepath.Join(os.DevNull, "termizard", "config.toml")
	if err := config.EnsureDefaultFile(path); err == nil {
		t.Fatal("expected error when config path parent is not a directory")
	}
}
