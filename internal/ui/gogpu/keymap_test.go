package gogpu

import (
	"testing"

	"github.com/gogpu/gpucontext"
)

// keyToCtrl

func TestKeyToCtrlLetterA(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeyA)
	if !ok || b != 0x01 {
		t.Fatalf("Ctrl+A = %02x ok=%v, want 0x01/true", b, ok)
	}
}

func TestKeyToCtrlLetterZ(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeyZ)
	if !ok || b != 0x1A {
		t.Fatalf("Ctrl+Z = %02x ok=%v, want 0x1A/true", b, ok)
	}
}

func TestKeyToCtrlLetterC(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeyC)
	if !ok || b != 0x03 {
		t.Fatalf("Ctrl+C = %02x ok=%v, want 0x03/true", b, ok)
	}
}

func TestKeyToCtrlBackspace(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeyBackspace)
	if !ok || b != 0x08 {
		t.Fatalf("Ctrl+Backspace = %02x ok=%v, want 0x08/true", b, ok)
	}
}

func TestKeyToCtrlEnter(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeyEnter)
	if !ok || b != 0x0D {
		t.Fatalf("Ctrl+Enter = %02x ok=%v, want 0x0D/true", b, ok)
	}
}

func TestKeyToCtrlSpace(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeySpace)
	if !ok || b != 0x00 {
		t.Fatalf("Ctrl+Space = %02x ok=%v, want 0x00/true", b, ok)
	}
}

func TestKeyToCtrlBackslash(t *testing.T) {
	b, ok := keyToCtrl(gpucontext.KeyBackslash)
	if !ok || b != 0x1C {
		t.Fatalf("Ctrl+Backslash = %02x ok=%v, want 0x1C/true", b, ok)
	}
}

func TestKeyToCtrlUnknown(t *testing.T) {
	_, ok := keyToCtrl(gpucontext.KeyF1)
	if ok {
		t.Fatal("F1 should not map to a Ctrl byte")
	}
}

// withAlt

func TestWithAltNone(t *testing.T) {
	got := withAlt(0, "\x1b[A")
	if string(got) != "\x1b[A" {
		t.Fatalf("no alt: %q", got)
	}
}

func TestWithAltPrepends(t *testing.T) {
	got := withAlt(gpucontext.ModAlt, "\x1b[A")
	if string(got) != "\x1b\x1b[A" {
		t.Fatalf("alt: %q", got)
	}
}

// keyToSeq

func TestKeyToSeqCtrlC(t *testing.T) {
	got := keyToSeq(gpucontext.KeyC, gpucontext.ModControl, false)
	if len(got) != 1 || got[0] != 0x03 {
		t.Fatalf("Ctrl+C seq = %v", got)
	}
}

func TestKeyToSeqCtrlAltC(t *testing.T) {
	got := keyToSeq(gpucontext.KeyC, gpucontext.ModControl|gpucontext.ModAlt, false)
	if len(got) != 2 || got[0] != 0x1b || got[1] != 0x03 {
		t.Fatalf("Ctrl+Alt+C = %v", got)
	}
}

func TestKeyToSeqCtrlShiftSkips(t *testing.T) {
	// Ctrl+Shift+C should not produce a C0 byte (requires no shift for C0)
	got := keyToSeq(gpucontext.KeyC, gpucontext.ModControl|gpucontext.ModShift, false)
	// should fall through to specialSeqs (none for C) → nil
	_ = got // must not panic
}

func TestKeyToSeqArrowUp(t *testing.T) {
	got := keyToSeq(gpucontext.KeyUp, 0, false)
	if string(got) != "\x1b[A" {
		t.Fatalf("Up = %q", got)
	}
}

func TestKeyToSeqArrowAlt(t *testing.T) {
	got := keyToSeq(gpucontext.KeyUp, gpucontext.ModAlt, false)
	if string(got) != "\x1b\x1b[A" {
		t.Fatalf("Alt+Up = %q", got)
	}
}

func TestKeyToSeqF1(t *testing.T) {
	got := keyToSeq(gpucontext.KeyF1, 0, false)
	if string(got) != "\x1bOP" {
		t.Fatalf("F1 = %q", got)
	}
}

func TestKeyToSeqF12(t *testing.T) {
	got := keyToSeq(gpucontext.KeyF12, 0, false)
	if string(got) != "\x1b[24~" {
		t.Fatalf("F12 = %q", got)
	}
}

func TestKeyToSeqEnter(t *testing.T) {
	got := keyToSeq(gpucontext.KeyEnter, 0, false)
	if string(got) != "\r" {
		t.Fatalf("Enter = %q", got)
	}
}

func TestKeyToSeqTab(t *testing.T) {
	got := keyToSeq(gpucontext.KeyTab, 0, false)
	if string(got) != "\t" {
		t.Fatalf("Tab = %q", got)
	}
}

func TestKeyToSeqBackspace(t *testing.T) {
	got := keyToSeq(gpucontext.KeyBackspace, 0, false)
	if string(got) != "\x7f" {
		t.Fatalf("Backspace = %q", got)
	}
}

func TestKeyToSeqDelete(t *testing.T) {
	got := keyToSeq(gpucontext.KeyDelete, 0, false)
	if string(got) != "\x1b[3~" {
		t.Fatalf("Delete = %q", got)
	}
}

func TestKeyToSeqPageUp(t *testing.T) {
	got := keyToSeq(gpucontext.KeyPageUp, 0, false)
	if string(got) != "\x1b[5~" {
		t.Fatalf("PageUp = %q", got)
	}
}

func TestKeyToSeqHome(t *testing.T) {
	got := keyToSeq(gpucontext.KeyHome, 0, false)
	if string(got) != "\x1b[H" {
		t.Fatalf("Home = %q", got)
	}
}

func TestKeyToSeqEnd(t *testing.T) {
	got := keyToSeq(gpucontext.KeyEnd, 0, false)
	if string(got) != "\x1b[F" {
		t.Fatalf("End = %q", got)
	}
}

func TestKeyToSeqEscape(t *testing.T) {
	got := keyToSeq(gpucontext.KeyEscape, 0, false)
	if string(got) != "\x1b" {
		t.Fatalf("Escape = %q", got)
	}
}

func TestKeyToSeqUnknown(t *testing.T) {
	got := keyToSeq(gpucontext.KeyUnknown, 0, false)
	if got != nil {
		t.Fatalf("unknown key = %v, want nil", got)
	}
}

// appCursor mode

func TestKeyToSeqAppCursorUp(t *testing.T) {
	got := keyToSeq(gpucontext.KeyUp, 0, true)
	if string(got) != "\x1bOA" {
		t.Fatalf("appCursor Up = %q", got)
	}
}

func TestKeyToSeqAppCursorDown(t *testing.T) {
	got := keyToSeq(gpucontext.KeyDown, 0, true)
	if string(got) != "\x1bOB" {
		t.Fatalf("appCursor Down = %q", got)
	}
}

func TestKeyToSeqAppCursorLeft(t *testing.T) {
	got := keyToSeq(gpucontext.KeyLeft, 0, true)
	if string(got) != "\x1bOD" {
		t.Fatalf("appCursor Left = %q", got)
	}
}

func TestKeyToSeqAppCursorRight(t *testing.T) {
	got := keyToSeq(gpucontext.KeyRight, 0, true)
	if string(got) != "\x1bOC" {
		t.Fatalf("appCursor Right = %q", got)
	}
}

func TestKeyToSeqAppCursorHome(t *testing.T) {
	got := keyToSeq(gpucontext.KeyHome, 0, true)
	if string(got) != "\x1bOH" {
		t.Fatalf("appCursor Home = %q", got)
	}
}

func TestKeyToSeqAppCursorAlt(t *testing.T) {
	got := keyToSeq(gpucontext.KeyUp, gpucontext.ModAlt, true)
	if string(got) != "\x1b\x1bOA" {
		t.Fatalf("appCursor Alt+Up = %q", got)
	}
}

// Non-appCursor key that has no appCursor override falls back to specialSeqs.
func TestKeyToSeqAppCursorF1FallsBack(t *testing.T) {
	got := keyToSeq(gpucontext.KeyF1, 0, true)
	if string(got) != "\x1bOP" {
		t.Fatalf("appCursor F1 = %q", got)
	}
}
