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

	"github.com/termizard/termizard/internal/util/logger"
)

const procThreadAttributePseudoConsole uintptr = 0x00020016

type startupInfoEx struct {
	windows.StartupInfo
	lpAttributeList unsafe.Pointer
}

type windowsPTY struct {
	hPC      windows.Handle
	rPipe    *os.File
	wPipe    *os.File
	proc     windows.Handle
	thread   windows.Handle
	pid      uint32
	attrList *windows.ProcThreadAttributeListContainer
	once     sync.Once
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

	var hPC windows.Handle
	coord := windows.Coord{X: int16(cfg.Cols), Y: int16(cfg.Rows)}
	if err := windows.CreatePseudoConsole(coord, childInRead, childOutWrite, 0, &hPC); err != nil {
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childOutWrite)
		windows.CloseHandle(childInRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: CreatePseudoConsole: %w", err)
	}
	windows.CloseHandle(childInRead)
	windows.CloseHandle(childOutWrite)

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
		nil, cmdLine, nil, nil, false,
		creationFlags, envBlock, dirUTF16,
		(*windows.StartupInfo)(unsafe.Pointer(&siex)),
		&procInfo,
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(childOutRead)
		windows.CloseHandle(childInWrite)
		return nil, fmt.Errorf("pty: CreateProcess %q: %w", command[0], err)
	}
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

func (p *windowsPTY) Read(buf []byte) (int, error)  { return p.rPipe.Read(buf) }
func (p *windowsPTY) Write(buf []byte) (int, error) { return p.wPipe.Write(buf) }
func (p *windowsPTY) Fd() uintptr                   { return uintptr(p.rPipe.Fd()) }
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
	logger.Get().Info("pty child exited", slog.Uint64("pid", uint64(p.pid)))
	return err
}

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
		return cmd, nil
	}
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return []string{comspec}, nil
	}
	return []string{`C:\Windows\System32\cmd.exe`}, nil
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
	block := strings.Join(env, "\x00") + "\x00\x00"
	return windows.UTF16PtrFromString(block)
}
