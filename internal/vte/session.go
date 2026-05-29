package vte

import (
	"errors"
	"io"
)

// Session wires an io.Reader (typically a pty.PTY) to a VTE Parser.
// It reads raw bytes from the reader, feeds them through the state machine,
// and calls Notify after each chunk so the caller can schedule a redraw.
//
// Usage:
//
//	sess := vte.NewSession(ptyDev, performer)
//	sess.Notify = func() { app.RequestRedraw() }
//	go func() { _ = sess.Run() }()
//	// later:
//	sess.Close()
type Session struct {
	r      io.Reader
	parser *Parser
	perf   Performer

	// Notify is called after each PTY chunk is parsed (i.e. when new output
	// arrived and the screen may have changed). Safe to set before Run().
	Notify func()

	closeOnce chan struct{} // closed by Close to signal Run to stop
}

// NewSession creates a Session that reads from r, parses with a fresh Parser,
// and dispatches sequences to perf.
func NewSession(r io.Reader, perf Performer) *Session {
	return &Session{
		r:         r,
		parser:    New(),
		perf:      perf,
		closeOnce: make(chan struct{}),
	}
}

// Parser returns the underlying Parser so callers can inspect state or swap it.
func (s *Session) Parser() *Parser { return s.parser }

// Run reads from the PTY in a blocking loop until the reader returns an error
// or Close is called. It is intended to be launched in a goroutine.
//
// Returns nil on clean EOF, or the read error otherwise.
func (s *Session) Run() error {
	buf := make([]byte, 32*1024)
	for {
		// Check for close signal (non-blocking).
		select {
		case <-s.closeOnce:
			return nil
		default:
		}

		n, err := s.r.Read(buf)
		if n > 0 {
			s.parser.Advance(s.perf, buf[:n])
			if s.Notify != nil {
				s.Notify()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// Close signals the Run loop to stop. It does not close the underlying reader;
// callers should close the PTY directly (which will unblock the Read call).
func (s *Session) Close() {
	select {
	case <-s.closeOnce:
		// already closed
	default:
		close(s.closeOnce)
	}
}
