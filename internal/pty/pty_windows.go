//go:build windows

package pty

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/termizard/termizard/internal/logger"
)

// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE is the attribute ID passed to
// UpdateProcThreadAttribute to associate a ConPTY handle with a new process.
const procThreadAttributePseudoConsole uintptr = 0x00020016

// startupInfoEx mirrors STARTUPINFOEXW.
// Windows reads past the base STARTUPINFOW when EXTENDED_STARTUPINFO_PRESENT
// is set in dwCreationFlags, using cb to confirm the larger size.
type startupInfoEx struct {
	windows.StartupInfo
	lpAttributeList unsafe.Pointer // *PROC_THREAD_ATTRIBUTE_LIST
}

// windowsPTY implements PTY via the Windows ConPTY API (CreatePseudoConsole).
// Available on Windows 10 version 1809 (build 17763) and later.
//
// I/O flow:
//
//	parent write → childIn write pipe → ConPTY → child stdin
//	child stdout → ConPTY → childOut read pipe → parent read
type windowsPTY struct {
	hPC      windows.Handle // pseudo console handle
	rPipe    *os.File       // parent reads child output from here
	wPipe    *os.File       // parent writes child input here
	proc     windows.Handle // child process handle
	thread   windows.Handle // child thread handle (closed after Start)
	pid      uint32
	attrList *windows.ProcThreadAttributeListContainer
	once     sync.Once // guards Close
}

func Open(cfg Config) (PTY, error) {
	command, err := resolveCommand(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("pty: %w", err)
	}
	dir, err := resolveDir(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("pty: %w", err)
	}
	env := cfg.Env
	if env == nil {
		env = os.Environ()
	}

	// Pipes: parent↔ConPTY (outer ends) and ConPTY↔child (inner ends, owned by ConPTY).
	var childOutRead, childOutWrite windows.Handle
	if err := windows.CreatePipe(&childOutRead, &childOutWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("pty: CreatePipe(out): %w", err)
	}
	var childInRead, childInWrite windows.Handle
	if err := windows.CreatePipe(&childInRead, &childInWrite, nil, 0); err != nil {
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childOutWrite)
		return nil, fmt.Errorf("pty: CreatePipe(in): %w", err)
	}

	// CreatePseudoConsole: the ConPTY owns childInRead (its stdin) and childOutWrite (its stdout).
	var hPC windows.Handle
	coord := windows.Coord{X: int16(cfg.Cols), Y: int16(cfg.Rows)}
	if err := windows.CreatePseudoConsole(coord, childInRead, childOutWrite, 0, &hPC); err != nil {
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childOutWrite)
		windows.CloseHandle(childInRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: CreatePseudoConsole: %w", err)
	}
	// ConPTY now owns the inner ends; close them in the parent.
	windows.CloseHandle(childInRead)
	windows.CloseHandle(childOutWrite)

	// Build a PROC_THREAD_ATTRIBUTE_LIST that attaches the ConPTY to the new process.
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: NewProcThreadAttributeList: %w", err)
	}
	if err := attrList.Update(
		procThreadAttributePseudoConsole,
		unsafe.Pointer(&hPC),
		unsafe.Sizeof(hPC),
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: UpdateProcThreadAttribute: %w", err)
	}

	// Build STARTUPINFOEX. cb must reflect the full extended struct size so Windows
	// reads the attribute list pointer that follows the base STARTUPINFOW.
	var siex startupInfoEx
	siex.Cb = uint32(unsafe.Sizeof(siex))
	siex.lpAttributeList = unsafe.Pointer(attrList.List())

	cmdLine, err := windows.UTF16PtrFromString(makeCmdLine(command))
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: encode cmdline: %w", err)
	}

	dirUTF16, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: encode dir: %w", err)
	}

	envBlock, err := makeEnvBlock(env)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: encode env: %w", err)
	}

	var procInfo windows.ProcessInformation
	creationFlags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(
		nil,
		cmdLine,
		nil,
		nil,
		false, // ConPTY uses attribute list, not inherited handles
		creationFlags,
		envBlock,
		dirUTF16,
		(*windows.StartupInfo)(unsafe.Pointer(&siex)), // cast: StartupInfoEx starts with StartupInfo
		&procInfo,
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: CreateProcess %q: %w", command[0], err)
	}
	// Thread handle is not needed after the process starts.
	windows.CloseHandle(procInfo.Thread)

	logger.Get().Info("pty opened",
		slog.String("cmd", command[0]),
		slog.Uint64("pid", uint64(procInfo.ProcessId)),
		slog.Uint64("cols", uint64(cfg.Cols)),
		slog.Uint64("rows", uint64(cfg.Rows)),
	)

	return &windowsPTY{
		hPC:      hPC,
		rPipe:    os.NewFile(uintptr(childOutRead), "pty-out"),
		wPipe:    os.NewFile(uintptr(childInWrite), "pty-in"),
		proc:     procInfo.Process,
		thread:   windows.InvalidHandle,
		pid:      procInfo.ProcessId,
		attrList: attrList,
	}, nil
}

func (p *windowsPTY) Read(buf []byte) (int, error) {
	return p.rPipe.Read(buf)
}

func (p *windowsPTY) Write(buf []byte) (int, error) {
	return p.wPipe.Write(buf)
}

// Resize calls ResizePseudoConsole which delivers WM_SIZE / WINDOW_BUFFER_SIZE_EVENT
// to the child's console input automatically.
func (p *windowsPTY) Resize(cols, rows uint16) error {
	err := windows.ResizePseudoConsole(p.hPC, windows.Coord{X: int16(cols), Y: int16(rows)})
	if err != nil {
		logger.Get().Warn("pty resize failed",
			slog.Uint64("pid", uint64(p.pid)),
			slog.String("err", err.Error()),
		)
	} else {
		logger.Get().Debug("pty resized",
			slog.Uint64("pid", uint64(p.pid)),
			slog.Uint64("cols", uint64(cols)),
			slog.Uint64("rows", uint64(rows)),
		)
	}
	return err
}

// Fd returns the read-pipe handle cast to uintptr.
// On Windows there is no epoll/kqueue; callers should use IOCP or
// WaitForMultipleObjects on this handle instead.
func (p *windowsPTY) Fd() uintptr {
	return uintptr(p.rPipe.Fd())
}

// Pid returns the child process ID.
func (p *windowsPTY) Pid() int {
	return int(p.pid)
}

// Wait blocks until the child process exits.
func (p *windowsPTY) Wait() error {
	_, err := windows.WaitForSingleObject(p.proc, windows.INFINITE)
	logger.Get().Info("pty child exited", slog.Uint64("pid", uint64(p.pid)))
	return err
}

// Close terminates the child process and releases all ConPTY resources.
func (p *windowsPTY) Close() (err error) {
	p.once.Do(func() {
		if termErr := windows.TerminateProcess(p.proc, 1); termErr != nil {
			logger.Get().Warn("pty terminate failed",
				slog.Uint64("pid", uint64(p.pid)),
				slog.String("err", termErr.Error()),
			)
		}
		windows.CloseHandle(p.proc)
		p.rPipe.Close()
		p.wPipe.Close()
		// ClosePseudoConsole must come after the pipes are closed.
		windows.ClosePseudoConsole(p.hPC)
		if p.attrList != nil {
			p.attrList.Delete()
		}
		logger.Get().Info("pty closed", slog.Uint64("pid", uint64(p.pid)))
	})
	return nil
}

// --- Internal helpers ---

// resolveCommand returns the command slice, falling back to cmd.exe.
func resolveCommand(cmd []string) ([]string, error) {
	if len(cmd) > 0 {
		return cmd, nil
	}
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return []string{comspec}, nil
	}
	return []string{`C:\Windows\System32\cmd.exe`}, nil
}

// resolveDir returns the working directory, falling back to %USERPROFILE%.
func resolveDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		return profile, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("pty: cannot determine home dir: %w", err)
	}
	return u.HomeDir, nil
}

// makeCmdLine assembles a Windows command line string, quoting arguments that
// contain spaces or special characters (CommandLineToArgvW-compatible).
func makeCmdLine(args []string) string {
	var b strings.Builder
	for i, arg := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(quoteWindowsArg(arg))
	}
	return b.String()
}

func quoteWindowsArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, c := range s {
		switch c {
		case '\\':
			slashes++
		case '"':
			for ; slashes > 0; slashes-- {
				b.WriteString(`\\`)
			}
			b.WriteString(`\"`)
		default:
			for ; slashes > 0; slashes-- {
				b.WriteByte('\\')
			}
			b.WriteRune(c)
		}
	}
	for ; slashes > 0; slashes-- {
		b.WriteString(`\\`)
	}
	b.WriteByte('"')
	return b.String()
}

// makeEnvBlock converts a []string of "KEY=VALUE" pairs into the
// double-null-terminated UTF-16 block expected by CreateProcess when
// CREATE_UNICODE_ENVIRONMENT is set. Returns nil for an empty/nil slice
// (signals CreateProcess to inherit the current environment).
func makeEnvBlock(env []string) (*uint16, error) {
	if len(env) == 0 {
		return nil, nil
	}
	block := strings.Join(env, "\x00") + "\x00\x00"
	return windows.UTF16PtrFromString(block)
}

// Ensure windowsPTY satisfies the io.ReadWriteCloser subset used in tests.
var _ io.ReadWriter = (*windowsPTY)(nil)
