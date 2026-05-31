// Package adapter defines the boundary between the terminal core and any
// windowing/rendering frontend. All types crossing this boundary live here so
// that neither side needs to import the other directly.
package adapter

// KeyEvent carries a single keyboard event from the frontend to the core.
// Data contains the UTF-8 bytes / escape sequence to write to the PTY.
type KeyEvent struct {
	Data []byte
}

// ResizeEvent is sent by the frontend when the drawable area changes.
type ResizeEvent struct {
	Cols uint16
	Rows uint16
}

// Frontend is implemented by every windowing / rendering backend
// (e.g. internal/frontend/gogpu_frontend, internal/frontend/mock_frontend).
//
// Lifecycle:
//
//	fe := gogpu_frontend.New(cfg)
//	fe.OnKeyInput(func(e adapter.KeyEvent) { pty.Write(e.Data) })
//	fe.OnResize(func(e adapter.ResizeEvent)  { pty.Resize(e.Cols, e.Rows) })
//	go fe.Run()   // blocks until window closes
//	...
//	fe.Close()
type Frontend interface {
	// Run starts the event loop. Blocks until the window is closed or Close is
	// called. Must be called from the main thread on platforms that require it
	// (macOS, Windows).
	Run() error

	// OnKeyInput registers a callback invoked on the event-loop goroutine
	// whenever the user presses a key or pastes text.
	OnKeyInput(fn func(KeyEvent))

	// OnResize registers a callback invoked when the window or font size
	// changes, yielding the new terminal grid dimensions.
	OnResize(fn func(ResizeEvent))

	// Close tears down the window and unblocks Run.
	Close() error
}
