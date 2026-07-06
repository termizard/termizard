//go:build !windows

package pty

import "runtime"

func platformDefaultShell() string {
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}
