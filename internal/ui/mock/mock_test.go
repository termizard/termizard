package mock_test

import (
	"testing"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/ui/mock"
)

func TestMockLifecycle(t *testing.T) {
	m := mock.New()

	var gotKey adapter.KeyEvent
	var gotResize adapter.ResizeEvent
	m.OnKeyInput(func(ev adapter.KeyEvent) { gotKey = ev })
	m.OnResize(func(ev adapter.ResizeEvent) { gotResize = ev })

	done := make(chan error, 1)
	go func() { done <- m.Run() }()

	m.RequestRedraw()

	if n, err := m.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
	}
	if len(m.Written) != 1 || string(m.Written[0]) != "hello" {
		t.Fatalf("Written = %v, want [hello]", m.Written)
	}

	m.SimulateKey([]byte("x"))
	if string(gotKey.Data) != "x" {
		t.Fatalf("SimulateKey = %q, want x", gotKey.Data)
	}

	m.SimulateResize(120, 40)
	if gotResize.Cols != 120 || gotResize.Rows != 40 {
		t.Fatalf("SimulateResize = (%d,%d), want (120,40)", gotResize.Cols, gotResize.Rows)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
