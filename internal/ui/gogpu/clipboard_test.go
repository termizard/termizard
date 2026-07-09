package gogpu

import (
	"runtime"
	"testing"
)

func TestClipboardWriteEmpty(t *testing.T) {
	u, _ := testUI(t)
	if err := u.clipboardWrite(""); err != nil {
		t.Fatalf("empty write: %v", err)
	}
}

func TestClipboardWriteNativeRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pbcopy/pbpaste only on macOS in CI")
	}
	const text = "termizard-clipboard-test"
	if err := clipboardWriteNative(text); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := clipboardReadNative()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !clipboardEqual(got, text) {
		t.Fatalf("roundtrip = %q, want %q", got, text)
	}
}

func TestClipboardReadViaUI(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native clipboard on macOS")
	}
	u, _ := testUI(t)
	const text = "termizard-ui-clipboard"
	if err := clipboardWriteNative(text); err != nil {
		t.Fatal(err)
	}
	got, err := u.clipboardRead()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !clipboardEqual(got, text) {
		t.Fatalf("read = %q", got)
	}
}

func TestClipboardWriteViaUI(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native clipboard on macOS")
	}
	u, _ := testUI(t)
	const text = "termizard-ui-write"
	if err := u.clipboardWrite(text); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := clipboardReadNative()
	if err != nil {
		t.Fatal(err)
	}
	if !clipboardEqual(got, text) {
		t.Fatalf("got %q", got)
	}
}

func TestClipboardReadNativeUnsupported(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("only for unix without wl/xclip")
	}
	_, err := clipboardReadNative()
	if err == nil {
		t.Skip("clipboard helper available")
	}
}
