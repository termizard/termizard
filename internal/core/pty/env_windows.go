//go:build windows

package pty

// preparePlatformEnv normalizes the PTY child environment on Windows.
func preparePlatformEnv(m map[string]string) {
	// Git for Windows sets $SHELL to bash.exe in the parent process env.
	// ConPTY children must not inherit it — always use a native shell path.
	m["SHELL"] = DefaultShell()
}
