// Package mock_frontend provides a headless Frontend implementation for tests
// and CI environments where no real window or GPU is available.
package mock_frontend

import (
	"sync"

	"github.com/termizard/termizard/internal/adapter"
)

// MockFrontend is a no-op frontend that records calls and lets tests inject
// synthetic input events.
type MockFrontend struct {
	mu       sync.Mutex
	keyFn    func(adapter.KeyEvent)
	resizeFn func(adapter.ResizeEvent)
	done     chan struct{}
}

// New returns a MockFrontend ready for use in tests.
func New() *MockFrontend {
	return &MockFrontend{done: make(chan struct{})}
}

// Run blocks until Close is called. Safe to call from any goroutine.
func (m *MockFrontend) Run() error {
	<-m.done
	return nil
}

// Close unblocks Run.
func (m *MockFrontend) Close() error {
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
func (m *MockFrontend) OnKeyInput(fn func(adapter.KeyEvent)) {
	m.mu.Lock()
	m.keyFn = fn
	m.mu.Unlock()
}

// OnResize registers the callback invoked by SimulateResize.
func (m *MockFrontend) OnResize(fn func(adapter.ResizeEvent)) {
	m.mu.Lock()
	m.resizeFn = fn
	m.mu.Unlock()
}

// SimulateKey injects a synthetic key event, as if the user typed data.
func (m *MockFrontend) SimulateKey(data []byte) {
	m.mu.Lock()
	fn := m.keyFn
	m.mu.Unlock()
	if fn != nil {
		fn(adapter.KeyEvent{Data: data})
	}
}

// SimulateResize injects a synthetic resize event.
func (m *MockFrontend) SimulateResize(cols, rows uint16) {
	m.mu.Lock()
	fn := m.resizeFn
	m.mu.Unlock()
	if fn != nil {
		fn(adapter.ResizeEvent{Cols: cols, Rows: rows})
	}
}

// Ensure MockFrontend satisfies the Frontend interface at compile time.
var _ adapter.Frontend = (*MockFrontend)(nil)
