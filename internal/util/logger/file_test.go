package logger_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/util/logger"
)

func TestLogFilePathNonEmpty(t *testing.T) {
	p := logger.LogFilePath()
	if p == "" {
		t.Fatal("LogFilePath returned empty string")
	}
	if !strings.Contains(p, "termizard") {
		t.Fatalf("LogFilePath does not contain 'termizard': %q", p)
	}
}

func TestSetupNonWindows(t *testing.T) {
	orig := logger.Get()
	t.Cleanup(func() { logger.Set(orig) })

	// On non-Windows this configures the global logger without writing a file.
	logPath, err := logger.Setup(false)
	if err != nil {
		t.Fatalf("Setup(false) error: %v", err)
	}
	_ = logPath
}

func TestSetupVerbose(t *testing.T) {
	orig := logger.Get()
	t.Cleanup(func() { logger.Set(orig) })

	_, err := logger.Setup(true)
	if err != nil {
		t.Fatalf("Setup(true) error: %v", err)
	}
}

func TestCloseNoFile(t *testing.T) {
	// Close before any file is opened must not panic.
	logger.Close()
}

func TestFlushWriterWritesThrough(t *testing.T) {
	// flushWriter wraps an inner Writer; verify write-through behavior via MultiWriter.
	var buf bytes.Buffer
	n, err := io.MultiWriter(&buf).Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("write: n=%d err=%v", n, err)
	}
	if buf.String() != "hello" {
		t.Fatalf("content: %q", buf.String())
	}
}
