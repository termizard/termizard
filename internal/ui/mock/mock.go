// Package mock provides a headless UI implementation for tests and CI
// environments where no real window or display is available.
package mock

import (
	"sync"

	"github.com/termizard/termizard/internal/adapter"
)

// Mock is a no-op UI that records calls and lets tests inject synthetic events.
type Mock struct {
	mu       sync.Mutex
	keyFn    func(adapter.KeyEvent)
	resizeFn func(adapter.ResizeEvent)
	done     chan struct{}
	Written  [][]byte // accumulated Write calls, for test assertions
}

// New returns a Mock ready for use in tests.
func New() *Mock {
	return &Mock{done: make(chan struct{})}
}

// Run blocks until Close is called.
func (m *Mock) Run() error {
	<-m.done
	return nil
}

// Close unblocks Run.
func (m *Mock) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.done:
	default:
		close(m.done)
	}
	return nil
}

// OnKeyInput registers the callback invoked by SimulateKey.
func (m *Mock) OnKeyInput(fn func(adapter.KeyEvent)) {
	m.mu.Lock()
	m.keyFn = fn
	m.mu.Unlock()
}

// OnResize registers the callback invoked by SimulateResize.
func (m *Mock) OnResize(fn func(adapter.ResizeEvent)) {
	m.mu.Lock()
	m.resizeFn = fn
	m.mu.Unlock()
}

// RequestRedraw is a no-op.
func (m *Mock) RequestRedraw() {}

// Write records the data and returns immediately.
func (m *Mock) Write(data []byte) (int, error) {
	buf := make([]byte, len(data))
	copy(buf, data)
	m.mu.Lock()
	m.Written = append(m.Written, buf)
	m.mu.Unlock()
	return len(data), nil
}

// SimulateKey injects a synthetic key event, as if the user typed data.
func (m *Mock) SimulateKey(data []byte) {
	m.mu.Lock()
	fn := m.keyFn
	m.mu.Unlock()
	if fn != nil {
		fn(adapter.KeyEvent{Data: data})
	}
}

// SimulateResize injects a synthetic resize event.
func (m *Mock) SimulateResize(cols, rows uint16) {
	m.mu.Lock()
	fn := m.resizeFn
	m.mu.Unlock()
	if fn != nil {
		fn(adapter.ResizeEvent{Cols: cols, Rows: rows})
	}
}

var _ adapter.UI = (*Mock)(nil)
