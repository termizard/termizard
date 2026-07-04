# Phase 1 — Core: PTY + VTE

**Status:** Done  
**ADR ref:** ADR-001 Phase 1 (Weeks 1–3)

## Goal

Wire a POSIX/Windows PTY to the Paul Williams VTE state machine so raw terminal
output can be parsed into structured events. No rendering yet — output goes into
a `Performer` callback.

## Directory layout produced

```
internal/
├── util/
│   └── logger/          # atomic global slog wrapper (zero-cost when silent)
├── core/
│   ├── pty/
│   │   ├── pty.go               # PTY interface + Config
│   │   ├── pty_posix.go         # creack/pty wrapper for macOS/Linux
│   │   ├── pty_windows.go       # ConPTY (CreatePseudoConsole) for Windows
│   │   ├── pty_posix_test.go    # unit tests for resolveCommand/resolveDir
│   │   └── pty_integration_test.go # spawn /bin/sh, echo, resize, wait
│   └── vte/
│       ├── performer.go         # Performer interface (callbacks from parser)
│       ├── params.go            # zero-alloc CSI/DCS parameter accumulator
│       ├── parser.go            # Paul Williams table-driven VT500 state machine
│       ├── session.go           # wires io.Reader → Parser → Performer + Notify
│       └── parser_test.go       # 30+ table-driven parser tests
├── adapter/
│   └── frontend.go      # Frontend interface (KeyEvent, ResizeEvent)
└── frontend/
    └── mock_frontend/
        └── mock.go      # headless Frontend for tests (SimulateKey/SimulateResize)
```

## Step-by-step

### 1 — PTY (`internal/core/pty`)

**macOS/Linux** (`pty_posix.go`):
```go
import creackpty "github.com/creack/pty"

master, err := creackpty.StartWithSize(cmd, &creackpty.Winsize{
    Rows: cfg.Rows, Cols: cfg.Cols,
})
```

**Windows** (`pty_windows.go`):
```go
windows.CreatePseudoConsole(coord, childInRead, childOutWrite, 0, &hPC)
```

Resize delivers `SIGWINCH` (Unix) / `WM_SIZE` (Windows) to the child automatically.

### 2 — VTE parser (`internal/core/vte`)

```go
p := vte.New()
p.Advance(performer, rawBytes) // zero-alloc hot path
```

The `paramsBuf` in `params.go` accumulates CSI/DCS parameters on the stack — no
heap allocation per sequence in the common case.

### 3 — Session wiring

```go
sess := vte.NewSession(ptyDev, myPerformer)
sess.Notify = func() { requestRedraw() }
go sess.Run()
// ...
sess.Close()
```

### 4 — Adapter interface

```go
type Frontend interface {
    Run() error
    OnKeyInput(fn func(KeyEvent))
    OnResize(fn func(ResizeEvent))
    Close() error
}
```

### 5 — Mock frontend for tests

```go
fe := mock_frontend.New()
fe.OnKeyInput(func(e adapter.KeyEvent) { pty.Write(e.Data) })
fe.SimulateKey([]byte("ls\n"))
fe.SimulateResize(120, 40)
fe.Close()
```

## Commands

```bash
# Build
go build ./...

# All tests
go test ./... -count=1 -timeout 30s

# PTY integration only (needs /bin/sh)
go test ./internal/core/pty/... -v -run TestRead

# VTE parser only
go test ./internal/core/vte/... -v

# Race detector
go test -race ./...
```

## Acceptance criteria

- [x] `go build ./...` clean
- [x] PTY opens, reads output, resizes, closes — tests pass
- [x] VTE parser: `echo`, `ls --color`, CSI SGR, OSC 0 title — tests pass
- [x] `go test -race ./...` no data races
- [x] Logger is silent by default; `logger.Set(slog.Default())` enables output
