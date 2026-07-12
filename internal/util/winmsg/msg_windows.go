//go:build windows

package winmsg

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	messageBoxW = user32.NewProc("MessageBoxW")
)

const mbIconError = 0x00000010

// Error shows a native error dialog. No-op if the message is empty.
func Error(title, text string) {
	if text == "" {
		return
	}
	if title == "" {
		title = "termizard"
	}
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	msg, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	_, _, _ = messageBoxW.Call(0, uintptr(unsafe.Pointer(msg)), uintptr(unsafe.Pointer(t)), mbIconError)
}
