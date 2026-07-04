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
