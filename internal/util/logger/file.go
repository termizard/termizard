package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	logFile   *os.File
	logFileMu sync.Mutex
)

// flushWriter syncs the underlying file after each write so diagnostics survive abrupt exits.
type flushWriter struct {
	io.Writer
	f *os.File
}

func (w *flushWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if err == nil && w.f != nil {
		_ = w.f.Sync()
	}
	return n, err
}

// LogFilePath returns the default diagnostic log file path.
func LogFilePath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "termizard", "termizard.log")
}

// Setup configures the global logger. On Windows, always writes to LogFilePath
// and mirrors to stderr (visible in console-subsystem builds).
func Setup(verbose bool) (logPath string, err error) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	var w io.Writer = os.Stderr
	if runtime.GOOS == "windows" {
		logPath = LogFilePath()
		if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o750); mkErr != nil {
			return "", mkErr
		}
		f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // G304: path from os.UserConfigDir
		if openErr != nil {
			return "", openErr
		}
		logFileMu.Lock()
		logFile = f
		logFileMu.Unlock()
		w = &flushWriter{
			Writer: io.MultiWriter(os.Stderr, f),
			f:      f,
		}
		fmt.Fprintf(os.Stderr, "termizard: log file: %s\n", logPath)
	}

	Set(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	Get().Info("termizard starting", slog.String("log", logPath), slog.Bool("verbose", verbose))
	return logPath, nil
}

// Close flushes the diagnostic log file, if any.
func Close() {
	logFileMu.Lock()
	defer logFileMu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
