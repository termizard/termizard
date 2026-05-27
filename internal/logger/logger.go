package logger

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// nopHandler silently discards all log records.
// Enabled returns false so the caller skips message formatting entirely,
// making disabled logging effectively zero-cost.
type nopHandler struct{}

func (nopHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (nopHandler) Handle(context.Context, slog.Record) error { return nil }
func (nopHandler) WithAttrs([]slog.Attr) slog.Handler        { return nopHandler{} }
func (nopHandler) WithGroup(string) slog.Handler             { return nopHandler{} }

// ptr stores the active logger, accessed atomically so Set can be called
// concurrently with logging from any goroutine.
var ptr atomic.Pointer[slog.Logger]

func init() {
	ptr.Store(slog.New(nopHandler{}))
}

// Get returns the current global logger. All packages in termizard log through this.
func Get() *slog.Logger { return ptr.Load() }

// Set replaces the global logger. Pass nil to restore silent (no-op) behavior.
// Safe for concurrent use.
//
// Log levels used across the codebase:
//   - [slog.LevelDebug]: internal diagnostics (resize events, fd registration)
//   - [slog.LevelInfo]: important lifecycle events (pty opened/closed)
//   - [slog.LevelWarn]: non-fatal issues (process already exited, resize failures)
//
// Example:
//
//	// Enable info-level logging to stderr:
//	logger.Set(slog.Default())
//
//	// Enable debug-level logging for full diagnostics:
//	logger.Set(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	})))
func Set(l *slog.Logger) {
	if l == nil {
		l = slog.New(nopHandler{})
	}
	ptr.Store(l)
}

// With returns a derived logger with the given attributes pre-applied.
// Equivalent to Get().With(args...) — useful for sub-package loggers
// that always attach a "component" or "pid" field.
func With(args ...any) *slog.Logger {
	return ptr.Load().With(args...)
}
