package logger_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/util/logger"
)

func TestDefaultIsSilent(t *testing.T) {
	l := logger.Get()
	if l == nil {
		t.Fatal("Get returned nil")
	}
	for _, lvl := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if l.Enabled(context.Background(), lvl) {
			t.Errorf("default logger must be silent, but Enabled=true for %s", lvl)
		}
	}
}

func TestSetNilRestoresSilence(t *testing.T) {
	orig := logger.Get()
	defer logger.Set(orig)

	var buf bytes.Buffer
	logger.Set(slog.New(slog.NewTextHandler(&buf, nil)))
	logger.Set(nil)

	if logger.Get() == nil {
		t.Fatal("Get returned nil after Set(nil)")
	}
	if logger.Get().Enabled(context.Background(), slog.LevelError) {
		t.Error("after Set(nil) logger must be silent")
	}
}

func TestSetCustom(t *testing.T) {
	orig := logger.Get()
	defer logger.Set(orig)

	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Set(custom)

	if logger.Get() != custom {
		t.Error("Get did not return the logger passed to Set")
	}

	logger.Get().Info("hello logger", "key", "value")
	if !strings.Contains(buf.String(), "hello logger") {
		t.Errorf("custom logger did not capture output: %q", buf.String())
	}
}

func TestSetIsAtomicRoundtrip(t *testing.T) {
	orig := logger.Get()
	defer logger.Set(orig)

	var buf bytes.Buffer
	l1 := slog.New(slog.NewTextHandler(&buf, nil))
	l2 := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	logger.Set(l1)
	if logger.Get() != l1 {
		t.Error("expected l1 after first Set")
	}
	logger.Set(l2)
	if logger.Get() != l2 {
		t.Error("expected l2 after second Set")
	}
}

func TestWith(t *testing.T) {
	orig := logger.Get()
	defer logger.Set(orig)

	var buf bytes.Buffer
	logger.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	logger.With("component", "pty", "pid", 42).Info("test message")

	out := buf.String()
	if !strings.Contains(out, "component=pty") {
		t.Errorf("With attr 'component' not found in output: %q", out)
	}
	if !strings.Contains(out, "pid=42") {
		t.Errorf("With attr 'pid' not found in output: %q", out)
	}
}

func TestWithDoesNotMutateGlobal(t *testing.T) {
	orig := logger.Get()
	defer logger.Set(orig)

	var buf bytes.Buffer
	logger.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	_ = logger.With("component", "pty")

	buf.Reset()
	logger.Get().Info("no component here")
	if strings.Contains(buf.String(), "component=pty") {
		t.Error("With leaked attrs into the global logger")
	}
}
