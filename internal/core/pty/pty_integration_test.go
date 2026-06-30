//go:build darwin || linux

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
	ptyDefaultCols uint16 = 80
	ptyDefaultRows uint16 = 24
	ptyTimeout            = 5 * time.Second
)

func openTestPTY(t *testing.T, cfg pty.Config) pty.PTY {
	t.Helper()
	if cfg.Cols == 0 {
		cfg.Cols = ptyDefaultCols
	}
	if cfg.Rows == 0 {
		cfg.Rows = ptyDefaultRows
	}
	p, err := pty.Open(cfg)
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func drainPTY(p pty.PTY, timeout time.Duration) (string, bool) {
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

func TestOpenPidFd(t *testing.T) {
	p := openTestPTY(t, pty.Config{Command: []string{"/bin/sh"}})
	if p.Pid() <= 0 {
		t.Errorf("expected positive PID, got %d", p.Pid())
	}
	if p.Fd() == 0 {
		t.Error("Fd() returned 0")
	}
}

func TestClose(t *testing.T) {
	p := openTestPTY(t, pty.Config{Command: []string{"/bin/sh"}})
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestResize(t *testing.T) {
	p := openTestPTY(t, pty.Config{Command: []string{"/bin/sh"}})
	for _, tc := range []struct{ cols, rows uint16 }{{120, 40}, {200, 50}, {80, 24}} {
		if err := p.Resize(tc.cols, tc.rows); err != nil {
			t.Errorf("Resize(%d,%d): %v", tc.cols, tc.rows, err)
		}
	}
}

func TestReadOutput(t *testing.T) {
	p := openTestPTY(t, pty.Config{Command: []string{"/bin/sh", "-c", "echo hello-pty-test"}})
	out, ok := drainPTY(p, ptyTimeout)
	if !ok {
		t.Fatal("timeout reading PTY output")
	}
	if !strings.Contains(out, "hello-pty-test") {
		t.Errorf("expected 'hello-pty-test' in output: %q", out)
	}
}

func TestWrite(t *testing.T) {
	p := openTestPTY(t, pty.Config{Command: []string{"/bin/sh"}})

	outCh := make(chan string, 1)
	go func() {
		out, _ := drainPTY(p, ptyTimeout)
		outCh <- out
	}()

	time.Sleep(80 * time.Millisecond)
	if _, err := p.Write([]byte("echo write-test\n")); err != nil {
		t.Fatalf("Write(echo): %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := p.Write([]byte("exit\n")); err != nil {
		t.Logf("Write(exit): %v", err)
	}

	select {
	case out := <-outCh:
		if !strings.Contains(out, "write-test") {
			t.Errorf("expected 'write-test' in output: %q", out)
		}
	case <-time.After(ptyTimeout):
		t.Fatal("timeout waiting for shell to exit after write")
	}
}

func TestWait(t *testing.T) {
	p := openTestPTY(t, pty.Config{Command: []string{"/bin/sh", "-c", "exit 0"}})

	done := make(chan error, 1)
	go func() { done <- p.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("Wait returned (non-nil may be normal on this platform): %v", err)
		}
	case <-time.After(ptyTimeout):
		t.Fatal("Wait did not return after child exited")
	}
}

func TestOpenDefaultShell(t *testing.T) {
	p := openTestPTY(t, pty.Config{})
	if p.Pid() <= 0 {
		t.Errorf("expected positive PID from default shell, got %d", p.Pid())
	}
}
