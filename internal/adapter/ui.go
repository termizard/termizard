// Package adapter defines the boundary between the terminal core and any
// windowing/rendering UI backend. All types crossing this boundary live here so
// that neither side needs to import the other directly.
package adapter

// KeyEvent carries a single keyboard event from the UI to the core.
// Data contains the UTF-8 bytes / escape sequence to write to the PTY.
type KeyEvent struct {
	Data []byte
}

// ResizeEvent is sent by the UI when the drawable area changes.
type ResizeEvent struct {
	Cols uint16
	Rows uint16
}

// UI is implemented by every windowing / rendering backend
// (e.g. internal/ui/gogpu, internal/ui/mock).
//
// Lifecycle:
//
//	ui := gogpu.New(cfg)
//	ui.OnKeyInput(func(e adapter.KeyEvent) { pty.Write(e.Data) })
//	ui.OnResize(func(e adapter.ResizeEvent)  { pty.Resize(e.Cols, e.Rows) })
//	go ui.Run()   // blocks until window closes
//	...
//	ui.Close()
type UI interface {
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

	// RequestRedraw schedules a repaint. No-op for backends that self-render
	// (e.g. xterm.js repaints on every Write call).
	RequestRedraw()

	// Write forwards raw PTY output to the terminal display.
	// For xterm.js backends this emits a Wails event; for GPU backends it
	// feeds data into the VTE parser and schedules a redraw.
	Write(data []byte) (int, error)

	// NotifyShellError reports PTY startup or exit failures to the frontend
	// without closing the window.
	NotifyShellError(tabID int, err error)
}
