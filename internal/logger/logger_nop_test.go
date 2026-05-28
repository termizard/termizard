// White-box tests for nopHandler. Must use package logger (not logger_test)
// because nopHandler is unexported.
package logger

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestNopHandlerHandle(t *testing.T) {
	h := nopHandler{}
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Errorf("Handle returned unexpected error: %v", err)
	}
}

func TestNopHandlerWithAttrs(t *testing.T) {
	h := nopHandler{}
	got := h.WithAttrs([]slog.Attr{slog.String("k", "v")})
	if _, ok := got.(nopHandler); !ok {
		t.Errorf("WithAttrs should return nopHandler, got %T", got)
	}
}

func TestNopHandlerWithGroup(t *testing.T) {
	h := nopHandler{}
	got := h.WithGroup("grp")
	if _, ok := got.(nopHandler); !ok {
		t.Errorf("WithGroup should return nopHandler, got %T", got)
	}
}
