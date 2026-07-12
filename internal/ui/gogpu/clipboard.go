package gogpu

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const osWindows = "windows"

// clipboardWrite puts text on the system clipboard.
// On macOS prefer pbcopy — gogpu NSPasteboard writes often appear to succeed
// while leaving the system pasteboard unchanged.
func (u *UI) clipboardWrite(text string) error {
	if text == "" {
		return nil
	}
	if isMac {
		err := clipboardWriteNative(text)
		if err == nil {
			return nil
		}
		u.debugf("clipboard: pbcopy failed: %v, trying gogpu", err)
		return u.app.ClipboardWrite(text)
	}
	err := u.app.ClipboardWrite(text)
	if err == nil {
		return nil
	}
	u.debugf("clipboard: gogpu ClipboardWrite: %v", err)
	return clipboardWriteNative(text)
}

func clipboardEqual(a, b string) bool {
	na := strings.ReplaceAll(strings.ReplaceAll(a, "\r\n", "\n"), "\r", "\n")
	nb := strings.ReplaceAll(strings.ReplaceAll(b, "\r\n", "\n"), "\r", "\n")
	return na == nb
}

// clipboardRead returns text from the system clipboard.
func (u *UI) clipboardRead() (string, error) {
	if isMac {
		if text, err := clipboardReadNative(); err == nil && text != "" {
			return text, nil
		}
	}
	text, err := u.app.ClipboardRead()
	if err == nil && text != "" {
		return text, nil
	}
	native, nerr := clipboardReadNative()
	if nerr == nil && native != "" {
		return native, nil
	}
	if err != nil {
		return "", err
	}
	if nerr != nil {
		return "", nerr
	}
	return "", nil
}

func clipboardWriteNative(text string) error {
	var cmd *exec.Cmd
	switch {
	case isMac:
		cmd = exec.Command("pbcopy")
	case runtime.GOOS == osWindows:
		cmd = exec.Command("clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			return fmt.Errorf("no clipboard helper (wl-copy/xclip)")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func clipboardReadNative() (string, error) {
	var cmd *exec.Cmd
	switch {
	case isMac:
		cmd = exec.Command("pbpaste")
	case runtime.GOOS == osWindows:
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw")
	default:
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "-n")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else {
			return "", fmt.Errorf("no clipboard helper (wl-paste/xclip)")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
