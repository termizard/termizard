package config_test

import (
	"testing"

	"github.com/termizard/termizard/internal/util/config"
)

func TestDefaultsDelegatesToInternalConfig(t *testing.T) {
	cfg := config.Defaults()
	if cfg == nil {
		t.Fatal("Defaults returned nil")
	}
	if !cfg.Window.ShowTitleBar {
		t.Fatal("expected show_title_bar default true")
	}
}
