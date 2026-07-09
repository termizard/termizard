//go:build windows

package pty

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestMakeEnvBlockMultipleEntries(t *testing.T) {
	block, err := makeEnvBlock([]string{
		"TERM=xterm-256color",
		"HOME=C:\\Users\\test",
		"TERMIZARD_PS_PROFILE=C:\\cache\\profile.ps1",
	})
	if err != nil {
		t.Fatalf("makeEnvBlock: %v", err)
	}
	if block == nil {
		t.Fatal("expected non-nil env block")
	}

	// Walk the UTF-16 block: each entry is NUL-terminated, block ends with double NUL.
	// makeEnvBlock returns *uint16 (a Windows LPWSTR); use unsafe.Slice to index it.
	raw := unsafe.Slice(block, 4096)
	var entries []string
	start := 0
	for i := 0; i < 4096; i++ {
		if raw[i] == 0 {
			if i == start {
				break
			}
			entries = append(entries, windows.UTF16ToString(raw[start:i]))
			start = i + 1
			if raw[start] == 0 {
				break
			}
		}
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %v, want 3", entries)
	}
}
