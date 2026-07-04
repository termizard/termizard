package vte_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/core/vte"
)

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestSessionRunAndClose(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("hello")

	term := terminal.New(10, 5, 100, true)
	sess := vte.NewSession(&buf, term)
	if sess.Parser() == nil {
		t.Fatal("Parser() returned nil")
	}

	notify := 0
	sess.Notify = func() { notify++ }

	if err := sess.Run(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if notify == 0 {
		t.Fatal("expected Notify to be called")
	}

	sess.Close()
	sess.Close() // idempotent
}

func TestSessionCloseWhileBlockedOnRead(t *testing.T) {
	pr, pw := io.Pipe()
	term := terminal.New(10, 5, 100, true)
	sess := vte.NewSession(pr, term)

	done := make(chan error, 1)
	go func() { done <- sess.Run() }()

	sess.Close()
	_ = pw.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Run to exit")
	}
}

func TestSessionReadError(t *testing.T) {
	term := terminal.New(10, 5, 100, true)
	want := errors.New("read failed")
	sess := vte.NewSession(errReader{err: want}, term)

	if err := sess.Run(); !errors.Is(err, want) {
		t.Fatalf("Run() = %v, want %v", err, want)
	}
}
