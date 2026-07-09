package gogpu

import (
	"testing"

	"github.com/termizard/termizard/internal/config"
)

// titleAlignForPaint

func TestTitleAlignForPaintNilConfig(t *testing.T) {
	got := titleAlignForPaint(nil)
	if got != config.TitleAlignCenter {
		t.Fatalf("nil cfg: got %q, want center", got)
	}
}

func TestTitleAlignForPaintCenter(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = config.TitleAlignCenter
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignCenter {
		t.Fatalf("got %q, want center", got)
	}
}

func TestTitleAlignForPaintLeft(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = config.TitleAlignLeft
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignLeft {
		t.Fatalf("got %q, want left", got)
	}
}

func TestTitleAlignForPaintRight(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = config.TitleAlignRight
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignRight {
		t.Fatalf("got %q, want right", got)
	}
}

func TestTitleAlignForPaintUnknownDefaultsToCenter(t *testing.T) {
	cfg := &config.Config{}
	cfg.Window.TitlePosition = "diagonal"
	got := titleAlignForPaint(cfg)
	if got != config.TitleAlignCenter {
		t.Fatalf("unknown position: got %q, want center", got)
	}
}
