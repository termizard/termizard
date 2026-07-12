//go:build windows

package logger_test

import (
	"os"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/util/logger"
)

func TestSetupWindowsCreatesLogFile(t *testing.T) {
	orig := logger.Get()
	t.Cleanup(func() {
		logger.Close()
		logger.Set(orig)
	})

	path, err := logger.Setup(true)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if path == "" {
		t.Fatal("empty log path on Windows")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if !strings.Contains(path, "termizard.log") {
		t.Fatalf("path = %q", path)
	}
}
