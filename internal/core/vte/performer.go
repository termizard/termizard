package vte

// Performer receives callbacks from the VTE parser for each decoded terminal
// sequence. The contract mirrors go-vte / alacritty/vte.
//
// All slice arguments (params, intermediates, osc params) are owned by the
// parser and are only valid for the duration of the call. Implementors must
// copy any data they need to retain.
type Performer interface {
	// Print is called for every printable character (including wide Unicode).
	Print(r rune)

	// Execute is called for C0/C1 control bytes (e.g. 0x07 BEL, 0x08 BS,
	// 0x0A LF, 0x0D CR).
	Execute(b byte)

	// EscDispatch is called for two-character ESC sequences.
	// intermediates holds collected bytes from 0x20-0x2F (e.g. none, or '%').
	// final is the terminating byte (0x30-0x7E).
	EscDispatch(intermediates []byte, final byte)

	// CSI is called for CSI (Control Sequence Introducer) sequences.
	// params is a slice of sub-param slices; ';' separates params, ':' separates
	// sub-params within a param (e.g. "38:2:255:0:0" → [[38,2,255,0,0]]).
	// intermediates holds collected bytes (e.g. '?' for DEC private modes).
	// ignore is true when the sequence was malformed and should be discarded.
	// final is the dispatch byte (e.g. 'm', 'H', 'J').
	CSI(params [][]uint16, intermediates []byte, ignore bool, final rune)

	// OSC is called when an OSC (Operating System Command) string is complete.
	// params are the raw byte fields separated by ';' (e.g. "0", "My Title").
	// bellTerminated is true when the sequence was closed by BEL (0x07) rather
	// than ST (ESC \\ or 0x9C).
	OSC(params [][]byte, bellTerminated bool)

	// DCS is called when the DCS (Device Control String) final byte is received.
	// After this, DCSPut is called for each passthrough byte, then DCSUnhook.
	DCS(params [][]uint16, intermediates []byte, ignore bool, final rune)

	// DCSPut is called for each byte in the DCS passthrough body.
	DCSPut(b byte)

	// DCSUnhook is called when the DCS string terminates (ST or 0x9C).
	DCSUnhook()
}
