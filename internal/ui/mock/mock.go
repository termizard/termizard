// Package mock_frontend provides a headless Frontend implementation for tests
// and CI environments where no real window or GPU is available.
package mock

import (
	"sync"

	"github.com/termizard/termizard/internal/adapter"
)

// Mock is a no-op frontend that records calls and lets tests inject
// synthetic input events.
type Mock struct {
	mu       sync.Mutex
	keyFn    func(adapter.KeyEvent)
	resizeFn func(adapter.ResizeEvent)
	done     chan struct{}
}

// New returns a Mock ready for use in tests.
func New() *Mock {
	return &Mock{done: make(chan struct{})}
}

// Run blocks until Close is called. Safe to call from any goroutine.
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

// Ensure Mock satisfies the Frontend interface at compile time.
var _ adapter.UI = (*Mock)(nil)
