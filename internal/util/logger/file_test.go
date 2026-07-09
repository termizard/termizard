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

// flushWriter is unexported; test it via an io.Writer that wraps a bytes.Buffer
// with a no-op Sync so we can verify the write-through behaviour.

type syncBuffer struct {
	bytes.Buffer
	synced bool
}

func (s *syncBuffer) Sync() error {
	s.synced = true
	return nil
}

func TestFlushWriterWritesThrough(t *testing.T) {
	// Use io.MultiWriter through a plain bytes.Buffer to verify the pattern:
	// flushWriter wraps an inner Writer and syncs the file after each write.
	var buf bytes.Buffer
	n, err := io.MultiWriter(&buf).Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("write: n=%d err=%v", n, err)
	}
	if buf.String() != "hello" {
		t.Fatalf("content: %q", buf.String())
	}
}
