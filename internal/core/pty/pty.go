package pty

import "io"

type PTY interface {
	io.Reader // Read raw bytes from the child process (blocks)
	io.Writer // Write bytes to the child's stdin (keystrokes, escape sequences)

	// Resize notifies the kernel and child process (via SIGWINCH on Unix,
	// ResizePseudoConsole on Windows) about a new terminal size.
	Resize(cols, rows uint16) error

	// Fd returns the master file descriptor.
	// Used by the event loop to register with epoll/kqueue.
	Fd() uintptr

	// Pid returns the child process PID.
	Pid() int

	// Wait blocks until the child process exits.
	// Should be called in a goroutine; closing the PTY unblocks it.
	Wait() error

	// Close terminates the child process and releases all resources.
	Close() error
}

type Config struct {
	// Command is the program to run, e.g. []string{"/bin/bash", "--login"}.
	// If empty, the user's $SHELL is used (see DefaultShell).
	Command []string

	// Env is the environment for the child process.
	// If nil, the current process environment is inherited.
	Env []string

	// Dir is the working directory for the child.
	// Defaults to the user's home directory.
	Dir string

	Cols uint16 // Initial terminal width in columns
	Rows uint16 // Initial terminal height in rows
}
