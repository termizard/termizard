//go:build windows

package pty

import (
	"testing"

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
	var entries []string
	start := 0
	for i := 0; i < 4096; i++ {
		if block[i] == 0 {
			if i == start {
				break
			}
			entries = append(entries, windows.UTF16ToString(block[start:i]))
			start = i + 1
			if block[start] == 0 {
				break
			}
		}
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %v, want 3", entries)
	}
}
