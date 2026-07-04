package vte_test

import (
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/core/vte"
)

// recorder implements Performer and collects all events.
type recorder struct {
	prints   []rune
	executes []byte
	escs     []escEvent
	csis     []csiEvent
	oscs     []oscEvent
}

type escEvent struct {
	inter []byte
	final byte
}
type csiEvent struct {
	params [][]uint16
	inter  []byte
	ignore bool
	final  rune
}
type oscEvent struct {
	params         [][]byte
	bellTerminated bool
}

func (r *recorder) Print(ch rune)  { r.prints = append(r.prints, ch) }
func (r *recorder) Execute(b byte) { r.executes = append(r.executes, b) }
func (r *recorder) EscDispatch(inter []byte, final byte) {
	cp := make([]byte, len(inter))
	copy(cp, inter)
	r.escs = append(r.escs, escEvent{inter: cp, final: final})
}
func (r *recorder) CSI(params [][]uint16, inter []byte, ignore bool, final rune) {
	ps := make([][]uint16, len(params))
	for i, p := range params {
		ps[i] = append([]uint16{}, p...)
	}
	ic := make([]byte, len(inter))
	copy(ic, inter)
	r.csis = append(r.csis, csiEvent{params: ps, inter: ic, ignore: ignore, final: final})
}
func (r *recorder) OSC(params [][]byte, bell bool) {
	ps := make([][]byte, len(params))
	for i, p := range params {
		ps[i] = append([]byte{}, p...)
	}
	r.oscs = append(r.oscs, oscEvent{params: ps, bellTerminated: bell})
}
func (r *recorder) DCS(_ [][]uint16, _ []byte, _ bool, _ rune) {}
func (r *recorder) DCSPut(_ byte)                              {}
func (r *recorder) DCSUnhook()                                 {}

type dcsRecorder struct {
	recorder
	hooked bool
	puts   []byte
	unhook bool
}

func (d *dcsRecorder) DCS(_ [][]uint16, _ []byte, _ bool, _ rune) { d.hooked = true }
func (d *dcsRecorder) DCSPut(b byte)                              { d.puts = append(d.puts, b) }
func (d *dcsRecorder) DCSUnhook()                                 { d.unhook = true }

func parse(input string) *recorder {
	rec := &recorder{}
	p := vte.New()
	p.Advance(rec, []byte(input))
	return rec
}

// --- Ground / Print ---

func TestPrintASCII(t *testing.T) {
	rec := parse("hello")
	if got := string(rec.prints); got != "hello" {
		t.Fatalf("prints: want %q, got %q", "hello", got)
	}
}

func TestPrintUTF8(t *testing.T) {
	rec := parse("café")
	got := string(rec.prints)
	if got != "café" {
		t.Fatalf("prints: want %q, got %q", "café", got)
	}
}

func TestPrintUTF8Wide(t *testing.T) {
	// 日本語
	input := "日本語"
	rec := parse(input)
	if string(rec.prints) != input {
		t.Fatalf("wide chars: want %q, got %q", input, string(rec.prints))
	}
}

// --- Execute (C0 controls) ---

func TestExecuteLF(t *testing.T) {
	rec := parse("a\nb")
	if len(rec.executes) != 1 || rec.executes[0] != 0x0A {
		t.Fatalf("expected LF execute, got %v", rec.executes)
	}
	if string(rec.prints) != "ab" {
		t.Fatalf("prints around LF: %q", string(rec.prints))
	}
}

// --- ESC sequences ---

func TestEscDispatchRI(t *testing.T) {
	// ESC M = Reverse Index
	rec := parse("\x1bM")
	if len(rec.escs) != 1 {
		t.Fatalf("expected 1 ESC dispatch, got %d", len(rec.escs))
	}
	if rec.escs[0].final != 'M' || len(rec.escs[0].inter) != 0 {
		t.Fatalf("ESC dispatch wrong: %+v", rec.escs[0])
	}
}

func TestEscDispatchSaveCursor(t *testing.T) {
	// ESC 7 = DECSC (save cursor)
	rec := parse("\x1b7")
	if len(rec.escs) != 1 || rec.escs[0].final != '7' {
		t.Fatalf("ESC 7 not dispatched: %+v", rec.escs)
	}
}

// --- CSI sequences ---

func TestCSIReset(t *testing.T) {
	// CSI m → reset attributes, no params
	rec := parse("\x1b[m")
	if len(rec.csis) != 1 {
		t.Fatalf("expected 1 CSI, got %d", len(rec.csis))
	}
	ev := rec.csis[0]
	if ev.final != 'm' || len(ev.params) != 0 {
		t.Fatalf("CSI reset: %+v", ev)
	}
}

func TestCSISGRBold(t *testing.T) {
	// CSI 1m → bold
	rec := parse("\x1b[1m")
	if len(rec.csis) != 1 {
		t.Fatalf("expected 1 CSI, got %d", len(rec.csis))
	}
	ev := rec.csis[0]
	if ev.final != 'm' || len(ev.params) != 1 || ev.params[0][0] != 1 {
		t.Fatalf("CSI 1m: %+v", ev)
	}
}

func TestCSISGRMultipleParams(t *testing.T) {
	// CSI 1;32m → bold + green
	rec := parse("\x1b[1;32m")
	ev := rec.csis[0]
	if len(ev.params) != 2 || ev.params[0][0] != 1 || ev.params[1][0] != 32 {
		t.Fatalf("CSI 1;32m params: %+v", ev.params)
	}
}

func TestCSISGRTrueColor(t *testing.T) {
	// CSI 38:2:255:0:128m → TrueColor via sub-params
	rec := parse("\x1b[38:2:255:0:128m")
	ev := rec.csis[0]
	if len(ev.params) != 1 {
		t.Fatalf("expected 1 param, got %d: %v", len(ev.params), ev.params)
	}
	want := []uint16{38, 2, 255, 0, 128}
	for i, v := range want {
		if ev.params[0][i] != v {
			t.Fatalf("sub-param[%d]: want %d, got %d", i, v, ev.params[0][i])
		}
	}
}

func TestCSICursorMove(t *testing.T) {
	// CSI 10;20H → cursor position row 10 col 20
	rec := parse("\x1b[10;20H")
	ev := rec.csis[0]
	if ev.final != 'H' || len(ev.params) != 2 {
		t.Fatalf("CUP: %+v", ev)
	}
	if ev.params[0][0] != 10 || ev.params[1][0] != 20 {
		t.Fatalf("CUP params: %v", ev.params)
	}
}

func TestCSIDecPrivateMode(t *testing.T) {
	// CSI ?25h → show cursor (DEC private mode)
	rec := parse("\x1b[?25h")
	ev := rec.csis[0]
	if ev.final != 'h' || len(ev.inter) != 1 || ev.inter[0] != '?' {
		t.Fatalf("?25h: %+v", ev)
	}
	if ev.params[0][0] != 25 {
		t.Fatalf("?25h param: %v", ev.params)
	}
}

func TestCSIEmptyParams(t *testing.T) {
	// CSI ;2m → two params, first defaults to 0
	rec := parse("\x1b[;2m")
	ev := rec.csis[0]
	if len(ev.params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(ev.params), ev.params)
	}
	if ev.params[0][0] != 0 || ev.params[1][0] != 2 {
		t.Fatalf(";2m params: %v", ev.params)
	}
}

// --- OSC sequences ---

func TestOSCTitleBEL(t *testing.T) {
	// OSC 0;My Title BEL
	rec := parse("\x1b]0;My Title\x07")
	if len(rec.oscs) != 1 {
		t.Fatalf("expected 1 OSC, got %d", len(rec.oscs))
	}
	ev := rec.oscs[0]
	if !ev.bellTerminated {
		t.Fatal("expected bell terminated")
	}
	if string(ev.params[0]) != "0" || string(ev.params[1]) != "My Title" {
		t.Fatalf("OSC params: %q %q", ev.params[0], ev.params[1])
	}
}

func TestOSCTitleST(t *testing.T) {
	// OSC 2;Title ST (ST = ESC \)
	rec := parse("\x1b]2;Title\x1b\\")
	if len(rec.oscs) != 1 {
		t.Fatalf("expected 1 OSC, got %d", len(rec.oscs))
	}
	ev := rec.oscs[0]
	if ev.bellTerminated {
		t.Fatal("expected ST (not BEL) terminated")
	}
	if string(ev.params[1]) != "Title" {
		t.Fatalf("OSC title: %q", ev.params[1])
	}
}

// --- Sequence boundaries ---

func TestMixedSequences(t *testing.T) {
	// Print "AB", bold, print "CD", reset
	rec := parse("AB\x1b[1mCD\x1b[m")
	if string(rec.prints) != "ABCD" {
		t.Fatalf("prints: %q", string(rec.prints))
	}
	if len(rec.csis) != 2 {
		t.Fatalf("expected 2 CSI, got %d", len(rec.csis))
	}
}

func TestAnywhereCancelsMidCSI(t *testing.T) {
	// CAN (0x18) inside CSI → cancels it, goes to Ground; text after is printed
	rec := parse("\x1b[1\x18hello")
	// CSI is canceled — no CSI dispatch, just Execute(0x18) then Print
	if len(rec.csis) != 0 {
		t.Fatalf("CSI should be canceled: %v", rec.csis)
	}
	if string(rec.prints) != "hello" {
		t.Fatalf("prints after cancel: %q", string(rec.prints))
	}
}

func TestUTF8InvalidContinuation(t *testing.T) {
	rec := parse("\xC2A") // broken UTF-8 sequence
	if len(rec.prints) == 0 {
		t.Fatal("expected replacement or partial print")
	}
}

func TestDCSSequenceDispatches(t *testing.T) {
	d := &dcsRecorder{}
	p := vte.New()
	p.Advance(d, []byte("\x1bP1;2;3zdata\x1b\\"))
	if !d.hooked {
		t.Fatal("expected DCS hook")
	}
	if !d.unhook {
		t.Fatal("expected DCS unhook")
	}
}

func TestOSCStringTerminator(t *testing.T) {
	rec := parse("\x1b]0;Title\x1b\\")
	if len(rec.oscs) != 1 {
		t.Fatalf("oscs = %d", len(rec.oscs))
	}
	if string(rec.oscs[0].params[0]) != "0" || string(rec.oscs[0].params[1]) != "Title" {
		t.Fatalf("osc params = %v", rec.oscs[0].params)
	}
}

// --- Session ---

func TestSession(t *testing.T) {
	rec := &recorder{}
	input := "Hello\x1b[1mWorld\x1b[m"
	r := strings.NewReader(input)
	sess := vte.NewSession(r, rec)
	var notified int
	sess.Notify = func() { notified++ }
	if err := sess.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(rec.prints) != "HelloWorld" {
		t.Fatalf("session prints: %q", string(rec.prints))
	}
	if len(rec.csis) != 2 {
		t.Fatalf("session csis: %d", len(rec.csis))
	}
	if notified == 0 {
		t.Fatal("Notify never called")
	}
}
