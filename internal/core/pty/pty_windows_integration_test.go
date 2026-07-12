//go:build windows && integration

package pty_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/termizard/termizard/internal/core/pty"
)

const (
	winPTYDefaultCols uint16 = 80
	winPTYDefaultRows uint16 = 24
	winPTYTimeout            = 10 * time.Second
)

func openWindowsTestPTY(t *testing.T, cfg pty.Config) pty.PTY {
	t.Helper()
	if cfg.Cols == 0 {
		cfg.Cols = winPTYDefaultCols
	}
	if cfg.Rows == 0 {
		cfg.Rows = winPTYDefaultRows
	}
	p, err := pty.Open(cfg)
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func drainWindowsPTY(p pty.PTY, timeout time.Duration) (string, bool) {
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&buf, p)
	}()
	select {
	case <-done:
		return buf.String(), true
	case <-time.After(timeout):
		return buf.String(), false
	}
}

func TestWindowsOpenPidFd(t *testing.T) {
	p := openWindowsTestPTY(t, pty.Config{Command: []string{"cmd.exe", "/c", "echo", "ok"}})
	if p.Pid() <= 0 {
		t.Errorf("expected positive PID, got %d", p.Pid())
	}
	if p.Fd() == 0 {
		t.Error("Fd() returned 0")
	}
}

func TestWindowsReadOutput(t *testing.T) {
	p := openWindowsTestPTY(t, pty.Config{Command: []string{"cmd.exe", "/c", "echo hello-pty-test"}})
	out, ok := drainWindowsPTY(p, winPTYTimeout)
	if !ok {
		t.Fatal("timeout reading PTY output")
	}
	if !strings.Contains(out, "hello-pty-test") {
		t.Errorf("expected 'hello-pty-test' in output: %q", out)
	}
}

func TestWindowsWriteInteractive(t *testing.T) {
	p := openWindowsTestPTY(t, pty.Config{Command: []string{"cmd.exe", "/K"}})

	outCh := make(chan string, 1)
	go func() {
		out, _ := drainWindowsPTY(p, winPTYTimeout)
		outCh <- out
	}()

	time.Sleep(200 * time.Millisecond)
	if _, err := p.Write([]byte("echo write-test\r\n")); err != nil {
		t.Fatalf("Write(echo): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := p.Write([]byte("exit\r\n")); err != nil {
		t.Logf("Write(exit): %v", err)
	}

	select {
	case out := <-outCh:
		if !strings.Contains(out, "write-test") {
			t.Errorf("expected 'write-test' in output: %q", out)
		}
	case <-time.After(winPTYTimeout):
		t.Fatal("timeout waiting for shell output after write")
	}
}

func TestWindowsResize(t *testing.T) {
	p := openWindowsTestPTY(t, pty.Config{Command: []string{"cmd.exe", "/K"}})
	for _, tc := range []struct{ cols, rows uint16 }{{120, 40}, {80, 24}} {
		if err := p.Resize(tc.cols, tc.rows); err != nil {
			t.Errorf("Resize(%d,%d): %v", tc.cols, tc.rows, err)
		}
	}
}

func TestWindowsOpenDefaultShell(t *testing.T) {
	p := openWindowsTestPTY(t, pty.Config{})
	if p.Pid() <= 0 {
		t.Errorf("expected positive PID from default shell, got %d", p.Pid())
	}
}
