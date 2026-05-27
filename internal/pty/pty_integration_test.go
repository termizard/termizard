//go:build darwin || linux

package pty_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/termizard/termizard/internal/pty"
)

const (
	ptyDefaultCols uint16 = 80
	ptyDefaultRows uint16 = 24
	ptyTimeout            = 5 * time.Second
)

// openTestPTY opens a PTY and registers t.Cleanup(p.Close).
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

// drainPTY reads from p until EIO / EOF (child process exits) and returns
// the collected output. Must be called in a goroutine or after the child exits.
// On macOS, cmd.Wait() blocks until the PTY output has been fully drained,
// so callers must call drainPTY concurrently with the child process running.
func drainPTY(p pty.PTY, timeout time.Duration) (string, bool) {
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(&buf, p) // EIO/EOF on process exit is expected
	}()
	select {
	case <-done:
		return buf.String(), true
	case <-time.After(timeout):
		return buf.String(), false
	}
}

// TestOpenPidFd verifies that Open succeeds and returns a positive PID and Fd.
func TestOpenPidFd(t *testing.T) {
	p := openTestPTY(
		t, pty.Config{
			Command: []string{"/bin/sh"},
		},
	)
	if p.Pid() <= 0 {
		t.Errorf("expected positive PID, got %d", p.Pid())
	}
	if p.Fd() == 0 {
		t.Error("Fd() returned 0")
	}
}

// TestClose verifies that Close returns no error.
func TestClose(t *testing.T) {
	p := openTestPTY(
		t, pty.Config{
			Command: []string{"/bin/sh"},
		},
	)
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// t.Cleanup calls Close again — must not panic
}

// TestResize verifies that Resize does not return an error for reasonable dimensions.
func TestResize(t *testing.T) {
	p := openTestPTY(
		t, pty.Config{
			Command: []string{"/bin/sh"},
		},
	)
	for _, tc := range []struct{ cols, rows uint16 }{{120, 40}, {200, 50}, {80, 24}} {
		if err := p.Resize(tc.cols, tc.rows); err != nil {
			t.Errorf("Resize(%d,%d): %v", tc.cols, tc.rows, err)
		}
	}
}

// TestReadOutput spawns "echo hello-pty-test" and verifies the output.
// On macOS, cmd.Wait() blocks until the PTY is fully drained, so we drain
// first. The drain goroutine returns EIO/EOF when the child exits.
func TestReadOutput(t *testing.T) {
	p := openTestPTY(
		t, pty.Config{
			Command: []string{"/bin/sh", "-c", "echo hello-pty-test"},
		},
	)

	out, ok := drainPTY(p, ptyTimeout)
	if !ok {
		t.Fatal("timeout reading PTY output")
	}
	if !strings.Contains(out, "hello-pty-test") {
		t.Errorf("expected 'hello-pty-test' in output: %q", out)
	}
}

// TestWrite sends a command to the shell via Write and reads the echoed output.
func TestWrite(t *testing.T) {
	p := openTestPTY(
		t, pty.Config{
			Command: []string{"/bin/sh"},
		},
	)

	// drainPTY in the background — it unblocks when the shell exits.
	outCh := make(chan string, 1)
	go func() {
		out, _ := drainPTY(p, ptyTimeout)
		outCh <- out
	}()

	time.Sleep(80 * time.Millisecond) // let shell reach read(2) on its stdin
	if _, err := p.Write([]byte("echo write-test\n")); err != nil {
		t.Fatalf("Write(echo): %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := p.Write([]byte("exit\n")); err != nil {
		t.Logf("Write(exit): %v", err) // non-fatal: shell may have already exited
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

// TestWait verifies that Wait returns when the child exits cleanly.
// Using a shell built-in (exit) avoids the macOS PTY drain-first requirement.
func TestWait(t *testing.T) {
	p := openTestPTY(
		t, pty.Config{
			Command: []string{"/bin/sh", "-c", "exit 0"},
		},
	)

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

// TestOpenDefaultShell verifies that Open works without an explicit Command.
func TestOpenDefaultShell(t *testing.T) {
	p := openTestPTY(t, pty.Config{})
	if p.Pid() <= 0 {
		t.Errorf("expected positive PID from default shell, got %d", p.Pid())
	}
}
