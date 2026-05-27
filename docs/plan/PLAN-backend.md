# TERMizard — Backend Implementation Plan
## PTY · VTE · Terminal Model

```
Status:   ACTIVE
Scope:    Phases 1–3 only (no GPU, no window)
Ref:      Rio Terminal v0.4.6 (github.com/raphamorim/rio)
Repo:     github.com/termizard/termizard
```

---

## What "backend" means here

Everything that runs before a single pixel is drawn:

```
PTY (kernel) → raw bytes → VTE parser → terminal model (grid) → [][]Cell
                                                                     ↑
                                              this is the GPU renderer's input
```

The backend is complete when you can run `vim`, `htop`, `tmux` and get a
correct `[][]Cell` grid — with no renderer attached.

---

## Repository layout (backend only)

### `internal/` vs `pkg/` — decision

```
internal/    ← private implementation, compiler-enforced boundary
             Go will refuse to compile any import of internal/ from
             outside github.com/termizard/termizard.
             All terminal engine code lives here: pty, vte, terminal, renderer.

pkg/         ← (future) stable public API surface, if any
             e.g. a pkg/termizard package exposing Terminal as a reusable
             library for third-party embedding. Currently empty.

cmd/         ← binaries: cmd/termizard (main app), cmd/headless (test harness)
```

This mirrors Rio's approach: all crates are workspace-internal, no public
library surface is exported. The terminal engine is an implementation detail.



```
termizard/
├── go.mod
├── go.sum
│
├── internal/
│   │
│   ├── pty/                          Rio: teletypewriter
│   │   ├── pty.go                    PTY interface (platform-agnostic)
│   │   ├── pty_unix.go               openpty(3), forkpty, TIOCSWINSZ
│   │   ├── pty_windows.go            ConPTY (CreatePseudoConsole)
│   │   └── process.go                spawn shell, waitpid, SIGCHLD
│   │
│   ├── vte/                          Rio: copa (fork of alacritty/vte)
│   │   ├── parser.go                 thin wrapper around go-vte
│   │   ├── handler.go                Handler interface
│   │   └── sequences/
│   │       ├── sgr.go                SGR: colors + attributes
│   │       ├── cursor.go             CUP, CUU/D/F/B, HVP
│   │       ├── erase.go              ED, EL
│   │       ├── scroll.go             SU, SD, DECSTBM
│   │       ├── mode.go               SM/RM: ?1049, ?25, ?7, ?1000-1006
│   │       ├── osc.go                OSC 0/1/2 title, OSC 52 clipboard, OSC 8 hyperlinks
│   │       └── dcs.go                DCS hook (Sixel stub)
│   │
│   ├── grapheme/                     Rio: rio-grapheme-width
│   │   └── width.go                  EAW width, ZWJ, grapheme cluster iteration
│   │
│   └── terminal/                     Rio: rio-backend
│       ├── terminal.go               Terminal: owns everything, public API
│       ├── cell.go                   Cell{Char, Fg, Bg, Attrs, Width, Hyperlink}
│       ├── color.go                  Color: Named16 / Indexed256 / TrueColor
│       ├── attrs.go                  CellAttrs bitfield
│       ├── grid.go                   Grid[rows][cols]Cell
│       ├── cursor.go                 Cursor{Col, Row, Style, Visible, Blinking}
│       ├── screen.go                 primary ↔ alt screen
│       ├── scrollback.go             ring buffer
│       ├── selection.go              Cell / Line / Block / Semantic
│       ├── hyperlink.go              OSC 8 hyperlink table
│       ├── modes.go                  mode flags (DEC private + standard)
│       └── sixel.go                  Sixel data accumulator (stub → Phase 7)
│
├── cmd/
│   └── headless/
│       └── main.go                   CLI test harness: dump grid as text/JSON
│
└── internal/
    └── testdata/
        ├── vttest/                   vttest escape sequence corpus
        └── golden/                   expected grid snapshots (JSON)
```

---

## Dependencies

### Core — required, Zero CGO

| Package | Version | Why |
|---|---|---|
| `github.com/creack/pty` | v1.1.21 | PTY open/fork/resize — used by Docker, VS Code server. Battle-tested on Linux/macOS/BSDs |
| `github.com/aymanbagabas/go-vte` | v0.0.5 | Exact Go port of alacritty/vte. Same Paul Williams VT500 automaton as Rio's `copa`. Supports CSI/OSC/DCS/Hook |
| `github.com/rivo/uniseg` | v0.4.7 | Grapheme cluster width (EAW, ZWJ emoji, CJK wide chars). Direct analog of `rio-grapheme-width` |
| `github.com/BurntSushi/toml` | v1.4.0 | TOML config — mirrors Rio's config schema |
| `github.com/fsnotify/fsnotify` | v1.7.0 | Config hot-reload — mirrors `rio-notifier` |
| `golang.org/x/sys` | v0.21.0 | ConPTY on Windows, ioctl, epoll/kqueue |

### Dev / Test only

| Package | Why |
|---|---|
| `github.com/stretchr/testify` | assert/require in tests |
| `github.com/google/go-cmp` | deep struct comparison for Cell grid golden tests |

### go.mod

```
module github.com/termizard/termizard

go 1.23

require (
    github.com/aymanbagabas/go-vte  v0.0.5
    github.com/BurntSushi/toml      v1.4.0
    github.com/creack/pty           v1.1.21
    github.com/fsnotify/fsnotify    v1.7.0
    github.com/rivo/uniseg          v0.4.7
    golang.org/x/sys                v0.21.0
)

require (
    github.com/stretchr/testify     v1.9.0  // test
    github.com/google/go-cmp        v0.6.0  // test
)
```

---

## Phase 1 — PTY  `internal/pty`

**Goal:** spawn a shell, read its output, write keystrokes, handle resize.

### Interface

```go
// internal/pty/pty.go

type PTY interface {
    // Read raw bytes from the child process (blocks until data available)
    Read(p []byte) (int, error)

    // Write bytes to the child's stdin (keystrokes, escape sequences)
    Write(p []byte) (int, error)

    // Resize notifies the kernel and child via SIGWINCH
    Resize(cols, rows uint16) error

    // Fd returns the master fd (needed for epoll/kqueue registration)
    Fd() uintptr

    // Close cleans up master fd and terminates child
    Close() error

    // PID returns the child process pid
    PID() int
}

// Config for spawning a new PTY
type Config struct {
    Command []string          // e.g. []string{"/bin/bash", "--login"}
    Env     []string          // inherit + override
    Dir     string            // working directory (default: $HOME)
    Cols    uint16            // initial size
    Rows    uint16
}

func Open(cfg Config) (PTY, error)
```

### Unix implementation  `pty_unix.go`

```go
// Uses github.com/creack/pty under the hood

func Open(cfg Config) (PTY, error) {
    cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
    cmd.Env = cfg.Env
    cmd.Dir = cfg.Dir

    ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
        Rows: cfg.Rows, Cols: cfg.Cols,
    })
    // ...
    return &unixPTY{master: ptmx, cmd: cmd}, nil
}

// Resize sends SIGWINCH automatically via creack/pty
func (p *unixPTY) Resize(cols, rows uint16) error {
    return pty.Setsize(p.master, &pty.Winsize{Rows: rows, Cols: cols})
}
```

### Windows implementation  `pty_windows.go`

```go
// Uses golang.org/x/sys/windows for ConPTY

func Open(cfg Config) (PTY, error) {
    // CreatePseudoConsole → CreateProcess with PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE
    // Input/output via anonymous pipes
}
```

### Process lifecycle  `process.go`

```go
// Shell exit detection — close the tab cleanly
func (p *unixPTY) Wait() error {
    return p.cmd.Wait()   // blocks; caller runs in goroutine
}

// SIGCHLD is handled by os/exec automatically via Wait()
```

### Test gate

```bash
go test ./internal/pty/... -v
# ✓ spawn bash, write "echo hello\n", read output, find "hello"
# ✓ resize to 40x10, TIOCGWINSZ confirms new size
# ✓ close PTY → child exits
```

---

## Phase 2 — VTE Parser  `internal/vte`

**Goal:** translate raw bytes from PTY into `Handler` method calls.
We wrap `go-vte` (the Go port of alacritty/vte) instead of writing the
state machine from scratch — same as Rio wraps the Rust vte crate in copa.

### Handler interface

```go
// internal/vte/handler.go

// Handler receives decoded terminal actions.
// Implemented by internal/terminal.Terminal.
type Handler interface {
    // Printable Unicode character — most frequent call
    Print(r rune)

    // C0/C1 control byte (BS=0x08, HT=0x09, LF=0x0A, CR=0x0D, BEL=0x07…)
    Execute(b byte)

    // CSI sequence: ESC [ <params> <intermediates> <final>
    // e.g. SGR: params=[1,31] final='m'
    CSI(params [][]uint16, intermediates []byte, final byte)

    // OSC sequence: ESC ] <n> ; <data> ST
    OSC(identifier []byte, data []byte)

    // DCS sequences (Sixel, DECRQSS…)
    DCSHook(params [][]uint16, intermediates []byte, final byte)
    DCSPut(b byte)
    DCSUnhook()
}
```

### Parser wrapper

```go
// internal/vte/parser.go

type Parser struct {
    inner   *govte.Parser   // aymanbagabas/go-vte
    handler Handler
}

func New(h Handler) *Parser {
    p := &Parser{handler: h}
    p.inner = govte.New(&dispatchAdapter{h: h})
    return p
}

// Parse is the hot path — called with every chunk from PTY.
// No allocations in the steady state.
func (p *Parser) Parse(data []byte) {
    p.inner.Advance(data)
}
```

### Sequence dispatch  `internal/vte/sequences/`

Each file handles one group of sequences, all called from
`terminal.Terminal.CSI()` / `terminal.Terminal.OSC()`:

```
sgr.go     — SGR (m): colors Named16/256/TrueColor, Bold/Italic/Under/Strike/Blink/Dim/Reverse
cursor.go  — CUU/CUD/CUF/CUB (A-D), CUP/HVP (H/f), CNL/CPL, CHA, VPA
erase.go   — ED (J 0/1/2/3), EL (K 0/1/2), ECH (X), DCH (P), ICH (@)
scroll.go  — SU/SD (S/T), IL/DL (L/M), DECSTBM (r) scroll region
mode.go    — SM/RM: ?1049 altscreen, ?25 cursor, ?7 wraparound,
             ?1000/1002/1003/1006 mouse, ?2004 bracketed paste
osc.go     — OSC 0/1/2 window title, OSC 52 clipboard, OSC 8 hyperlinks
dcs.go     — DCS accumulator → Sixel stub (full impl in Phase 7)
```

### Sequence priority for MVP

```
MUST (Phase 2):
  SGR    m      colors + all attributes
  CUU    A      cursor up
  CUD    B      cursor down
  CUF    C      cursor forward
  CUB    D      cursor back
  CUP    H      cursor position
  HVP    f      cursor position (alias)
  ED     J      erase display  0/1/2/3
  EL     K      erase line     0/1/2
  SU     S      scroll up
  SD     T      scroll down
  IL     L      insert lines
  DL     M      delete lines
  ICH    @      insert characters
  DCH    P      delete characters
  ECH    X      erase characters
  RI     ESC M  reverse index
  DECSTBM r     scroll region
  SM/RM  ?1049  alternate screen
  SM/RM  ?25    cursor visibility
  SM/RM  ?7     auto-wrap mode
  OSC 0/1/2     window title
  OSC 52        clipboard

PHASE 3 (mouse + links):
  SM/RM ?1000/1002/1003/1006  mouse reporting
  SM/RM ?2004                 bracketed paste
  OSC 8                       hyperlinks
```

### Test gate

```bash
go test ./internal/vte/... -v -run TestSGR
go test ./internal/vte/... -v -run TestCursor
# run vttest corpus:
go run ./cmd/headless -- vttest < ./internal/testdata/vttest/cursor.vt
```

---

## Phase 3 — Terminal Model  `internal/terminal`

**Goal:** maintain the correct grid state as VTE events arrive.

### Cell

```go
// internal/terminal/cell.go

type Cell struct {
    Char      rune        // primary character (0 = empty)
    Width     uint8       // 1 normal, 2 wide CJK/emoji, 0 continuation
    Fg        Color
    Bg        Color
    Attrs     CellAttrs
    Hyperlink uint32      // index into Terminal.hyperlinks, 0 = none
}

// internal/terminal/attrs.go
type CellAttrs uint16

const (
    AttrBold      CellAttrs = 1 << iota
    AttrDim
    AttrItalic
    AttrUnderline
    AttrUndercurl
    AttrBlink
    AttrReverse
    AttrInvisible
    AttrStrike
    AttrDblUnder
)
```

### Color

```go
// internal/terminal/color.go

type ColorKind uint8
const (
    ColorDefault  ColorKind = iota
    ColorNamed               // 0–15 ANSI
    ColorIndexed             // 0–255
    ColorRGB
)

type Color struct {
    Kind ColorKind
    R, G, B uint8
    Index   uint8
}

var DefaultFg = Color{Kind: ColorDefault}
var DefaultBg = Color{Kind: ColorDefault}
```

### Grid

```go
// internal/terminal/grid.go

type Grid struct {
    cols    int
    rows    int
    cells   [][]Cell    // [rows][cols]  — row-major flat alloc
    dirty   []bool      // dirty[row] = true when line needs re-render
}

func NewGrid(cols, rows int) *Grid

// Line returns row i as a slice (no copy).
func (g *Grid) Line(row int) []Cell

// Set writes a cell and marks the row dirty.
func (g *Grid) Set(col, row int, c Cell)

// Resize reallocates the grid, preserving content where possible.
func (g *Grid) Resize(cols, rows int)

// Clear fills a rectangle with the given cell (used by ED/EL).
func (g *Grid) Clear(x0, y0, x1, y1 int, fill Cell)

// ScrollUp shifts lines [top, bot] up by n, fills bottom with blank.
func (g *Grid) ScrollUp(top, bot, n int, fill Cell)

// ScrollDown shifts lines [top, bot] down by n, fills top with blank.
func (g *Grid) ScrollDown(top, bot, n int, fill Cell)
```

### Scrollback Buffer

```go
// internal/terminal/scrollback.go

// Ring buffer — zero allocations after initial setup.
type Scrollback struct {
    lines    [][]Cell   // ring
    head     int        // index of oldest line
    count    int        // lines currently stored
    capacity int        // default 10 000
}

func (s *Scrollback) Push(line []Cell)          // called on scroll-up
func (s *Scrollback) Line(i int) []Cell         // 0 = most recent
func (s *Scrollback) Len() int
```

### Cursor

```go
// internal/terminal/cursor.go

type CursorStyle uint8
const (
    CursorBlock    CursorStyle = iota
    CursorUnderline
    CursorBar
)

type Cursor struct {
    Col, Row int
    Style    CursorStyle
    Visible  bool
    Blinking bool
    // Saved cursor (DECSC/DECRC)
    savedCol, savedRow int
    savedAttrs         CellAttrs
}
```

### Screen  (primary ↔ alt)

```go
// internal/terminal/screen.go

type Screen struct {
    grid      *Grid
    cursor    Cursor
    scrollTop int
    scrollBot int    // inclusive, default = rows-1
}

type Terminal struct {
    primary   Screen
    alt       Screen
    active    *Screen       // points to primary or alt

    scrollback *Scrollback
    hyperlinks []string      // OSC 8 URL table

    modes     ModeSet
    title     string
    mu        sync.RWMutex  // grid protected for renderer access
}
```

### Terminal public API

```go
// internal/terminal/terminal.go

func New(cols, rows int) *Terminal

// Grid returns a read-locked view of the current cell grid.
// Caller must call Unlock() when done.
func (t *Terminal) Grid() (*Grid, func())

// Resize adapts grid and scrollback to new dimensions.
func (t *Terminal) Resize(cols, rows int)

// --- Handler implementation (called by internal/vte) ---
func (t *Terminal) Print(r rune)
func (t *Terminal) Execute(b byte)
func (t *Terminal) CSI(params [][]uint16, intermediates []byte, final byte)
func (t *Terminal) OSC(id, data []byte)
func (t *Terminal) DCSHook(params [][]uint16, intermediates []byte, final byte)
func (t *Terminal) DCSPut(b byte)
func (t *Terminal) DCSUnhook()
```

### Selection Engine

```go
// internal/terminal/selection.go

type SelectionMode uint8
const (
    SelectNone     SelectionMode = iota
    SelectCell                   // normal drag
    SelectLine                   // triple-click / shift-click line
    SelectBlock                  // Alt+drag rectangle
    SelectSemantic               // double-click word/URL
)

type Selection struct {
    Mode  SelectionMode
    Start Point
    End   Point
}

type Point struct{ Col, Row int }

func (s *Selection) Contains(col, row int) bool
func (t *Terminal) SelectedText() string
```

### Modes

```go
// internal/terminal/modes.go

type ModeSet struct {
    AltScreen        bool
    CursorVisible    bool
    AutoWrap         bool
    BracketedPaste   bool
    MouseButton      bool   // ?1000
    MouseDrag        bool   // ?1002
    MouseMotion      bool   // ?1003
    MouseSGR         bool   // ?1006
    ApplicationCursor bool  // ?1
}
```

### Test gate

```bash
go test ./internal/terminal/... -v

# integration: run real programs headlessly
go run ./cmd/headless -- -c "echo hello"       # basic output
go run ./cmd/headless -- -c "ls --color"        # SGR colors
go run ./cmd/headless -- -c "vim /dev/null"     # altscreen + cursor movement
go run ./cmd/headless -- -c "htop"              # full TUI

# vttest
go run ./cmd/headless -- vttest
```

---

## Headless Test Harness  `cmd/headless`

```go
// cmd/headless/main.go
//
// Runs a command in a PTY, collects the grid after 500ms,
// dumps it as coloured ANSI text (for eyeballing) and JSON (for golden tests).
//
// Usage:
//   go run ./cmd/headless -c "ls --color" -cols 80 -rows 24 -json

func main() {
    // 1. Open PTY with given command
    // 2. Pipe output through internal/vte.Parser
    // 3. Parser calls internal/terminal.Terminal (implements Handler)
    // 4. After timeout/exit, call terminal.Grid()
    // 5. Print grid as text or JSON
}
```

---

## Test Strategy

### Unit tests (per package)

```
internal/pty/         spawn, read, write, resize, close
internal/vte/         every sequence in sequences/ has table-driven tests
internal/terminal/    grid ops (scroll, resize, clear), cursor movement,
                 altscreen switch, selection, scrollback ring
```

### Golden tests

```
internal/testdata/golden/
  ls-color.json     expected grid after "ls --color"
  vim-empty.json    expected grid after "vim /dev/null" (altscreen)
  htop-init.json    expected grid after htop startup frame
```

Run with:
```bash
go test ./... -update   # regenerate golden files
go test ./...           # compare against golden
```

### vttest corpus

```bash
# Download vttest and run through headless harness
./vttest 2>&1 | go run ./cmd/headless --vtmode
```

### Fuzz targets

```go
// internal/vte/fuzz_test.go
func FuzzParse(f *testing.F) {
    f.Add([]byte("\x1b[1;31m"))
    f.Fuzz(func(t *testing.T, data []byte) {
        p := vte.New(&noopHandler{})
        p.Parse(data)  // must not panic
    })
}
```

---

## Acceptance Criteria (backend complete)

The backend is "done" when all of these pass:

```
[ ] go test ./... passes on Linux, macOS, Windows
[ ] vttest Phase 1 (cursor movement)     — 100%
[ ] vttest Phase 2 (screen features)     — 100%
[ ] vttest Phase 3 (character sets)      — ≥ 80%
[ ] "ls --color" golden test             — exact match
[ ] "vim /dev/null" golden test          — altscreen correct
[ ] "htop" renders without corruption after 1s
[ ] "tmux" opens a pane, cursor in correct position
[ ] go test -race ./...                  — no data races
[ ] FuzzParse 5 minutes                  — no panics
[ ] RSS of headless binary               — < 15 MB
[ ] PTY read throughput                  — > 150 MB/s (go bench)
```

---

## What comes next (not in this plan)

```
Phase 4  internal/renderer  — Sugar glyph atlas, two-pass GPU pipeline (gogpu)
Phase 5  internal/app       — gogpu window, event loop, keybindings
Phase 6  internal/config    — TOML + fsnotify hot-reload, tabs, splits
Phase 7  Task manager  — bubbletea overlay, gopsutil metrics
```

---

## References

| Resource | URL |
|---|---|
| Rio Terminal (blueprint) | https://github.com/raphamorim/rio |
| Rio: teletypewriter | https://github.com/raphamorim/rio/tree/main/teletypewriter |
| Rio: copa (VTE) | https://github.com/raphamorim/rio/tree/main/copa |
| Rio: rio-backend | https://github.com/raphamorim/rio/tree/main/rio-backend |
| go-vte (our VTE lib) | https://github.com/aymanbagabas/go-vte |
| creack/pty | https://github.com/creack/pty |
| rivo/uniseg | https://github.com/rivo/uniseg |
| Paul Williams VT500 | https://vt100.net/emu/dec_ansi_parser |
| vttest | https://invisible-island.net/vttest/ |
