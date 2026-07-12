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
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/termizard/termizard/internal/util/logger"
)

const procThreadAttributePseudoConsole uintptr = 0x00020016

var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var procPeekNamedPipe = kernel32.NewProc("PeekNamedPipe")

func peekNamedPipe(handle windows.Handle, totalBytesAvail *uint32) error {
	r1, _, err := procPeekNamedPipe.Call(
		uintptr(handle),
		0, 0, 0,
		uintptr(unsafe.Pointer(totalBytesAvail)),
		0,
	)
	if r1 == 0 {
		return err
	}
	return nil
}

type windowsPTY struct {
	hPC      windows.Handle
	rHandle  windows.Handle // childOutRead — we read PTY output from here
	wPipe    *os.File       // childInWrite — we send keystrokes here
	proc     windows.Handle
	pid      uint32
	attrList *windows.ProcThreadAttributeListContainer
	once     sync.Once
	closedCh chan struct{}
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
	env := PrepareShellEnv(cfg.Env)

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

	cleanupPipes := func() {
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childOutWrite)
		windows.CloseHandle(childInRead)
		windows.CloseHandle(childInWrite)
	}

	// Pass HPCON by value to UpdateProcThreadAttribute (MS EchoCon / go-pty form).
	// Passing &hPC (pointer-to-handle) causes STATUS_DLL_INIT_FAILED (0xC0000142).
	var hPC windows.Handle
	coord := windows.Coord{X: int16(cfg.Cols), Y: int16(cfg.Rows)}
	if err := windows.CreatePseudoConsole(coord, childInRead, childOutWrite, 0, &hPC); err != nil {
		cleanupPipes()
		return nil, fmt.Errorf("pty: CreatePseudoConsole: %w", err)
	}

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		cleanupPipes()
		return nil, fmt.Errorf("pty: NewProcThreadAttributeList: %w", err)
	}
	if err := attrList.Update(
		procThreadAttributePseudoConsole,
		unsafe.Pointer(hPC),
		unsafe.Sizeof(hPC),
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		cleanupPipes()
		return nil, fmt.Errorf("pty: UpdateProcThreadAttribute: %w", err)
	}

	siEx := &windows.StartupInfoEx{}
	siEx.Cb = uint32(unsafe.Sizeof(*siEx))
	// Null std handles + STARTF_USESTDHANDLES prevents the child from inheriting
	// the parent's console handles (e.g. when launched via `go run` from PowerShell),
	// which conflicts with ConPTY attachment.
	siEx.Flags = windows.STARTF_USESTDHANDLES
	siEx.ProcThreadAttributeList = attrList.List()

	cmdLine, err := windows.UTF16PtrFromString(makeCmdLine(command))
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		cleanupPipes()
		return nil, fmt.Errorf("pty: encode cmdline: %w", err)
	}

	dirUTF16, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		cleanupPipes()
		return nil, fmt.Errorf("pty: encode dir: %w", err)
	}

	envBlock, err := makeEnvBlock(env)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		cleanupPipes()
		return nil, fmt.Errorf("pty: encode env: %w", err)
	}

	// CREATE_NO_WINDOW / DETACHED_PROCESS / CREATE_NEW_CONSOLE are forbidden with ConPTY.
	creationFlags := uint32(
		windows.EXTENDED_STARTUPINFO_PRESENT |
			windows.CREATE_UNICODE_ENVIRONMENT,
	)
	var procInfo windows.ProcessInformation
	if err := windows.CreateProcess(
		nil, cmdLine, nil, nil, false,
		creationFlags, envBlock, dirUTF16,
		&siEx.StartupInfo,
		&procInfo,
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		cleanupPipes()
		return nil, fmt.Errorf("pty: CreateProcess %q: %w", command[0], err)
	}
	windows.CloseHandle(procInfo.Thread)

	// MS sample: close the pipe ends given to CreatePseudoConsole after the child starts.
	// ConPTY has duplicated them; keeping our copies open is unnecessary.
	windows.CloseHandle(childInRead)
	windows.CloseHandle(childOutWrite)

	if err := windows.ResizePseudoConsole(hPC, coord); err != nil {
		logger.Get().Warn("pty initial resize failed",
			slog.Uint64("pid", uint64(procInfo.ProcessId)),
			slog.String("err", err.Error()),
		)
	}

	logger.Get().Info("pty opened",
		slog.String("cmd", command[0]),
		slog.String("argv", makeCmdLine(command)),
		slog.Uint64("pid", uint64(procInfo.ProcessId)),
		slog.Uint64("cols", uint64(cfg.Cols)),
		slog.Uint64("rows", uint64(cfg.Rows)),
	)

	return &windowsPTY{
		hPC:      hPC,
		rHandle:  childOutRead,
		wPipe:    os.NewFile(uintptr(childInWrite), "pty-in"),
		proc:     procInfo.Process,
		pid:      procInfo.ProcessId,
		attrList: attrList,
		closedCh: make(chan struct{}),
	}, nil
}

// Read polls the output pipe using PeekNamedPipe and reads when data is
// available. Polling avoids a blocking ReadFile call that would prevent
// clean shutdown via closedCh.
func (p *windowsPTY) Read(buf []byte) (int, error) {
	const pollMs = 5
	for {
		select {
		case <-p.closedCh:
			return 0, io.EOF
		default:
		}

		var avail uint32
		if err := peekNamedPipe(p.rHandle, &avail); err != nil {
			if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_PIPE_NOT_CONNECTED {
				return 0, io.EOF
			}
			return 0, err
		}
		if avail > 0 {
			toRead := uint32(len(buf))
			if avail < toRead {
				toRead = avail
			}
			var n uint32
			if err := windows.ReadFile(p.rHandle, buf[:toRead], &n, nil); err != nil {
				if err == windows.ERROR_BROKEN_PIPE {
					return int(n), io.EOF
				}
				return int(n), err
			}
			return int(n), nil
		}

		time.Sleep(pollMs * time.Millisecond)
	}
}

func (p *windowsPTY) Write(buf []byte) (int, error) { return p.wPipe.Write(buf) }
func (p *windowsPTY) Fd() uintptr                   { return uintptr(p.rHandle) }
func (p *windowsPTY) Pid() int                      { return int(p.pid) }

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

func (p *windowsPTY) Wait() error {
	_, err := windows.WaitForSingleObject(p.proc, windows.INFINITE)
	var code uint32
	if codeErr := windows.GetExitCodeProcess(p.proc, &code); codeErr != nil {
		logger.Get().Info("pty child exited", slog.Uint64("pid", uint64(p.pid)))
	} else {
		logger.Get().Info("pty child exited",
			slog.Uint64("pid", uint64(p.pid)),
			slog.Uint64("exit_code", uint64(code)),
		)
	}
	return err
}

func (p *windowsPTY) Close() (err error) {
	p.once.Do(func() {
		close(p.closedCh)
		if termErr := windows.TerminateProcess(p.proc, 1); termErr != nil {
			logger.Get().Warn("pty terminate failed",
				slog.Uint64("pid", uint64(p.pid)),
				slog.String("err", termErr.Error()),
			)
		}
		windows.CloseHandle(p.proc)
		windows.CloseHandle(p.rHandle)
		p.wPipe.Close()
		windows.ClosePseudoConsole(p.hPC)
		if p.attrList != nil {
			p.attrList.Delete()
		}
		logger.Get().Info("pty closed", slog.Uint64("pid", uint64(p.pid)))
	})
	return nil
}

var _ io.ReadWriter = (*windowsPTY)(nil)

// --- Internal helpers ---

func resolveCommand(cmd []string) ([]string, error) {
	if len(cmd) > 0 {
		return normalizeWindowsCommand(cmd), nil
	}
	return windowsDefaultCommand(), nil
}

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

func makeEnvBlock(env []string) (*uint16, error) {
	if len(env) == 0 {
		return nil, nil
	}
	// Windows env blocks are UTF-16 key=value pairs separated by NUL bytes,
	// terminated by a double NUL. UTF16PtrFromString rejects embedded NULs.
	block := make([]uint16, 0, 256)
	for _, kv := range env {
		u16, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, fmt.Errorf("pty: encode env entry %q: %w", kv, err)
		}
		block = append(block, u16...)
	}
	block = append(block, 0)
	return &block[0], nil
}
