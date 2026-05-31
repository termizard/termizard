# Termizard — MVP Implementation Plan

**Stack:** Go 1.26 · gogpu (GPU frontend) · creack/pty · Paul Williams VTE parser  
**References:** kitty (keyboard/color), alacritty (simplicity/correctness), rio (Sugar renderer/GPU pipeline)  
**Target:** macOS + Linux. Windows ConPTY stub already in place.

---

## 1. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  cmd/termizard                                                  │
│  main() → app.New(cfg, frontend) → app.Run()                   │
└───────────┬─────────────────────────────────────────────────────┘
            │
┌───────────▼──────────────┐     ┌───────────────────────────────┐
│  internal/app            │     │  internal/util/config         │
│  App: wires all layers   │◄────│  TOML loader + defaults       │
└──┬──────────┬────────────┘     └───────────────────────────────┘
   │          │
   │  goroutine: PTY reader
   │          │
┌──▼──────┐  ┌▼────────────────┐
│core/pty │  │core/vte          │
│PTY      │─►│Session → Parser  │
│interface│  │→ Performer (CSI/ │
│POSIX /  │  │ESC/OSC/DCS)      │
│ConPTY   │  └────────┬─────────┘
└─────────┘           │ Print/CSI/ESC/OSC calls
                       │
              ┌────────▼─────────┐
              │core/terminal      │
              │Terminal (Performer│
              │impl):             │
              │ · Screen (Grid)   │
              │ · Scrollback      │
              │ · SGR decoder     │
              │ · Cursor state    │
              └────────┬──────────┘
                       │ dirty rows / frame request
                       │
              ┌────────▼──────────┐
              │renderer            │
              │ · GlyphAtlas       │
              │ · FontMetrics      │
              │ · Frame() →        │
              │   *image.RGBA      │
              └────────┬───────────┘
                       │ pixel buffer
                       │
              ┌────────▼───────────────────────────────────────────┐
              │  adapter.Frontend (interface)                       │
              │                                                     │
              │  ┌──────────────────┐   ┌──────────────────────┐  │
              │  │ gogpu_frontend   │   │ mock_frontend (tests) │  │
              │  │ Window + Texture │   │ SimulateKey/Resize    │  │
              │  │ upload + draw    │   └──────────────────────┘  │
              │  └──────────────────┘                              │
              └────────────────────────────────────────────────────┘
```

### Goroutine model

```
main goroutine       │ gogpu event loop (Run) — MUST stay on main thread
PTY reader goroutine │ reads bytes → vte.Session → Terminal (mutex-locked)
                     │ → atomic "dirty" flag → gogpu.RequestRedraw()
render call          │ inside gogpu OnDraw callback: lock Terminal, render frame,
                     │ upload RGBA texture, draw full-screen quad
```

---

## 2. Component Table

| Component | Package | Responsibility | Key types |
|---|---|---|---|
| PTY | `internal/core/pty` | spawn shell, resize, R/W | `PTY`, `Config` |
| VTE parser | `internal/core/vte` | byte stream → Performer callbacks | `Parser`, `Session`, `Performer` |
| Terminal model | `internal/core/terminal` | screen grid, SGR, cursor, scrollback | `Terminal`, `Screen`, `Grid`, `Cell`, `Color` |
| Renderer | `internal/renderer` | Cell grid → *image.RGBA, glyph atlas | `Renderer`, `Atlas`, `FontMetrics` |
| Frontend interface | `internal/adapter` | boundary type: key/resize events | `Frontend`, `KeyEvent`, `ResizeEvent` |
| gogpu frontend | `internal/frontend/gogpu_frontend` | window, GPU texture, key mapping | `GogpuFrontend` |
| Mock frontend | `internal/frontend/mock_frontend` | headless backend for tests | `MockFrontend` |
| App | `internal/app` | wires all layers, goroutine lifecycle | `App` |
| Config | `internal/util/config` | TOML loader, sane defaults | `Config` |
| Logger | `internal/util/logger` | zero-cost global slog wrapper | `Get`, `Set` |

---

## 3. Phases

### Phase 1 — PTY + VTE ✅ DONE
`internal/core/pty`, `internal/core/vte`, `internal/adapter`, `internal/frontend/mock_frontend`

---

### Phase 2 — Terminal Model
**Package:** `internal/core/terminal`  
**Effort:** 3–4 days  
**Ref:** rio `corcovado` crate + alacritty `grid` crate

#### 2.1 Types

```go
// cell.go
type ColorKind uint8
const (
    ColorDefault ColorKind = iota
    ColorANSI              // 0-15
    ColorIndexed           // 16-255
    ColorRGB               // 24-bit
)
type Color struct {
    Kind  ColorKind
    Value uint32 // ANSI index OR 0x00RRGGBB
}

type Attrs uint16
const (
    AttrBold          Attrs = 1 << 0
    AttrDim           Attrs = 1 << 1
    AttrItalic        Attrs = 1 << 2
    AttrUnderline     Attrs = 1 << 3
    AttrBlink         Attrs = 1 << 4
    AttrInverse       Attrs = 1 << 5
    AttrInvisible     Attrs = 1 << 6
    AttrStrikethrough Attrs = 1 << 7
)

type Cell struct {
    Char  rune
    Width uint8  // 1 = narrow, 2 = wide (CJK)
    FG    Color
    BG    Color
    Attrs Attrs
}
```

#### 2.2 Grid

```go
// grid.go
type Grid struct {
    cells []Cell  // flat: row*cols + col
    cols  int
    rows  int
    dirty []bool  // per-row dirty flag (renderer reads this)
}

func (g *Grid) Cell(row, col int) *Cell { return &g.cells[row*g.cols+col] }
func (g *Grid) MarkDirty(row int)       { g.dirty[row] = true }
func (g *Grid) ClearDirty()             { clear(g.dirty) }
func (g *Grid) IsDirty(row int) bool    { return g.dirty[row] }
func (g *Grid) Resize(cols, rows int)   { /* reflow: copy existing cells */ }
```

#### 2.3 Scrollback ring buffer

```go
// scrollback.go — ring buffer, zero allocations on push after initial fill
type Scrollback struct {
    lines  [][]Cell  // ring
    head   int
    count  int
    cap    int
}
func NewScrollback(lines int) *Scrollback { ... }
func (s *Scrollback) Push(line []Cell)    { ... } // called on scroll-up
func (s *Scrollback) Line(idx int) []Cell { ... } // idx 0 = most recent
```

#### 2.4 Screen

```go
// screen.go
type CursorState struct {
    Row, Col   int
    Visible    bool
    SavedRow, SavedCol int // DECSC/DECRC
}
type Screen struct {
    primary   *Grid
    alternate *Grid
    active    *Grid         // points to primary or alternate
    cursor    CursorState
    scrollTop int           // DECSTBM top (0-based)
    scrollBot int           // DECSTBM bottom
    // SGR pen
    fg, bg    Color
    attrs     Attrs
    // charset
    charset   uint8         // 0 or 1
}
```

#### 2.5 Terminal (implements vte.Performer)

```go
// terminal.go
type Terminal struct {
    mu       sync.RWMutex
    screen   Screen
    scroll   *Scrollback
    title    string
    onTitle  func(string)  // called on OSC 0/2
    onBell   func()
}

func (t *Terminal) Print(r rune)    { /* advance cursor, handle wide chars */ }
func (t *Terminal) Execute(b byte)  { /* LF, CR, BS, BEL, HT, SI, SO */ }
func (t *Terminal) CSI(params [][]uint16, inter []byte, ignore bool, final rune) {
    // Dispatch on final:
    // 'm' → SGR (colors + attrs)
    // 'H','f' → CUP cursor position
    // 'A','B','C','D' → CUU/CUD/CUF/CUB
    // 'E','F' → CNL/CPL
    // 'G' → CHA (cursor horizontal absolute)
    // 'J' → ED  (erase display)
    // 'K' → EL  (erase line)
    // 'L','M' → IL/DL (insert/delete lines)
    // 'P' → DCH (delete chars)
    // '@' → ICH (insert chars)
    // 'S','T' → SU/SD (scroll up/down)
    // 'r' → DECSTBM (scroll region)
    // 's','u' → DECSC/DECRC (save/restore cursor)
    // 'h','l' → private modes: ?1049 (alt screen), ?25 (cursor visibility),
    //           ?1 (DECCKM application cursor keys), ?12 (cursor blink)
}
func (t *Terminal) OSC(params [][]byte, bell bool) {
    // 0, 1, 2 → window title
}
```

#### 2.6 SGR decoder

```go
// sgr.go — called from Terminal.CSI when final == 'm'
// Full support: 16 colors, 256 colors (38;5;n / 48;5;n),
// true color (38;2;r;g;b / 48;2;r;g;b), attrs, reset.
```

#### Phase 2 acceptance criteria
- [ ] `echo -e "\033[1;32mHello\033[0m"` → bold green "Hello"
- [ ] `vim` opens, edits a file, closes without corruption
- [ ] `htop` renders: alt-screen switch, colors, borders
- [ ] `resize(cols, rows)` does not corrupt cell content
- [ ] scrollback: `cat /usr/share/dict/words` → can scroll up
- [ ] `go test -race ./internal/core/terminal/...` → clean

---

### Phase 3 — CPU Renderer
**Package:** `internal/renderer`  
**Effort:** 2–3 days  
**Ref:** alacritty `renderer` (CPU path), sugarloaf early version

CPU renderer writes *image.RGBA. Phase 4 uploads this as a gogpu texture.
Two-pass GPU pipeline (background quads + glyph quads) is Phase 4+.

#### 3.1 Font metrics

```go
// font.go
type FontMetrics struct {
    Face       font.Face
    CellWidth  int  // pixels
    CellHeight int  // pixels
    Ascent     int  // pixels from top to baseline
}
func LoadMetrics(size float64) (*FontMetrics, error)  // golang.org/x/image/font/basicfont or truetype
```

For MVP: `golang.org/x/image/font/basicfont.Face7x13` (zero external deps).
After MVP: load a real TrueType font via `golang.org/x/image/font/opentype`.

#### 3.2 Glyph atlas

```go
// atlas.go
// Shelf-packing: https://observablehq.com/@mourner/simple-rectangle-packing
type Atlas struct {
    Image   *image.RGBA          // CPU-side atlas texture
    glyphs  map[glyphKey]GlyphEntry
    shelf   int                   // current shelf Y
    cursor  int                   // current X on shelf
    shelfH  int                   // height of current shelf
}
type GlyphEntry struct { X, Y, W, H int } // rect in atlas
func (a *Atlas) Lookup(r rune, face font.Face) GlyphEntry
```

#### 3.3 Frame renderer

```go
// renderer.go
type Renderer struct {
    metrics *FontMetrics
    atlas   *Atlas
    buf     *image.RGBA  // reused each frame
}

func New(metrics *FontMetrics) *Renderer
func (r *Renderer) Resize(cols, rows int)
func (r *Renderer) Frame(t *terminal.Terminal) *image.RGBA {
    t.RLock(); defer t.RUnlock()
    screen := t.Screen()
    for row := 0; row < screen.Rows(); row++ {
        if !screen.Grid().IsDirty(row) {
            continue  // skip clean rows — same as rio's dirty-line optimization
        }
        r.renderRow(screen, row)
    }
    screen.Grid().ClearDirty()
    return r.buf
}
```

Dirty-row tracking means idle terminal (cursor blinking or nothing happening)
costs ~0 CPU for rendering — same pattern as kitty and rio.

#### Phase 3 acceptance criteria
- [ ] `renderer.Frame()` returns correct *image.RGBA for a 80×24 grid
- [ ] Only dirty rows are re-rendered (benchmark: steady-state < 0.5% CPU)
- [ ] Atlas does not corrupt glyphs at >256 unique chars
- [ ] Renderer is safe for concurrent read from terminal (RWMutex)

---

### Phase 4 — gogpu Frontend
**Package:** `internal/frontend/gogpu_frontend`  
**Effort:** 3–4 days  
**Ref:** ADR-001 §6, gogpu API

#### 4.1 Window + texture

```go
// frontend.go
type GogpuFrontend struct {
    cfg      *config.Config
    metrics  *renderer.FontMetrics
    terminal *terminal.Terminal
    rend     *renderer.Renderer

    win     *gogpu.Window   // set inside Run() on main thread
    tex     gogpu.Texture   // GPU texture, re-created on resize
    texW    int
    texH    int

    keyFn    func(adapter.KeyEvent)
    resizeFn func(adapter.ResizeEvent)
}

func (f *GogpuFrontend) Run() error {
    app := gogpu.NewApp()
    app.NewWindow(gogpu.WindowOptions{
        Title:  "termizard",
        Width:  f.cfg.Window.Width,
        Height: f.cfg.Window.Height,
    }, func(w *gogpu.Window) {
        f.win = w
        w.OnDraw(f.onDraw)
        w.OnKey(f.onKey)
        w.OnResize(f.onResized)
        w.OnChar(f.onChar)
    })
    return app.Run()
}
```

#### 4.2 Draw callback

```go
func (f *GogpuFrontend) onDraw(dc gogpu.DrawContext) {
    frame := f.rend.Frame(f.terminal)
    if f.tex == nil {
        f.tex = dc.Renderer().NewTextureFromRGBA(frame)
    } else {
        f.tex.UpdateData(frame.Pix)
    }
    fw, fh := dc.FramebufferSize()
    dc.DrawTextureScaled(f.tex, 0, 0, float32(fw), float32(fh))
}
```

#### 4.3 Key mapping (kitty-style)

```go
// keymap.go
// Map gogpu Key + Modifiers → ANSI escape sequence / UTF-8 bytes
// Mirrors kitty's terminfo entries.
var specialKeys = map[gpucontext.Key]string{
    gpucontext.KeyUp:        "\x1b[A",
    gpucontext.KeyDown:      "\x1b[B",
    gpucontext.KeyRight:     "\x1b[C",
    gpucontext.KeyLeft:      "\x1b[D",
    gpucontext.KeyHome:      "\x1b[H",
    gpucontext.KeyEnd:       "\x1b[F",
    gpucontext.KeyPageUp:    "\x1b[5~",
    gpucontext.KeyPageDown:  "\x1b[6~",
    gpucontext.KeyInsert:    "\x1b[2~",
    gpucontext.KeyDelete:    "\x1b[3~",
    gpucontext.KeyF1:        "\x1bOP",
    // ... F2-F12, shift/ctrl variants
}
```

Application cursor keys mode (`?1h` DECCKM): arrows send `\x1bOA` instead of `\x1b[A`.  
This must be tracked in `Terminal` and checked in `GogpuFrontend.onKey`.

#### 4.4 Resize

```go
func (f *GogpuFrontend) onResized(w, h int) {
    cols := w / f.metrics.CellWidth
    rows := h / f.metrics.CellHeight
    f.tex = nil  // force re-create at new size
    f.rend.Resize(cols, rows)
    if f.resizeFn != nil {
        f.resizeFn(adapter.ResizeEvent{Cols: uint16(cols), Rows: uint16(rows)})
    }
}
```

#### Phase 4 acceptance criteria
- [ ] Window opens, shell prompt visible
- [ ] Type characters → appear on screen
- [ ] Arrow keys work in vim
- [ ] Resize window → shell/vim reflown correctly
- [ ] Idle CPU < 1% (dirty-row optimization active)
- [ ] No texture corruption on HiDPI (2× display)

---

### Phase 5 — App + Config
**Packages:** `internal/app`, `internal/util/config`  
**Effort:** 2 days

#### 5.1 Config (TOML)

```toml
# ~/.config/termizard/config.toml

[font]
family  = ""          # "" = built-in 7×13 bitmap
size    = 14.0

[window]
width   = 800
height  = 600
padding = [4, 4]
opacity = 1.0

[scrollback]
lines = 10000

[cursor]
shape    = "block"    # block | beam | underline
blinking = false

[colors]
# Solarized Dark (example)
foreground = "#839496"
background = "#002b36"

[shell]
program = ""          # "" = $SHELL
args    = []

[keyboard]
# key bindings (override defaults)
# [[keyboard.bindings]]
# key  = "C"
# mods = ["Ctrl", "Shift"]
# action = "Copy"
```

```go
// config.go
type Config struct {
    Font       FontConfig
    Window     WindowConfig
    Scrollback ScrollbackConfig
    Cursor     CursorConfig
    Colors     ColorConfig
    Shell      ShellConfig
    Keyboard   KeyboardConfig
}
func Defaults() *Config      { ... }
func LoadFile(path string) (*Config, error) { ... } // BurntSushi/toml
func ConfigPath() string     { ... } // XDG_CONFIG_HOME/termizard/config.toml
```

#### 5.2 App

```go
// app.go
type App struct {
    cfg      *config.Config
    pty      pty.PTY
    session  *vte.Session
    terminal *terminal.Terminal
    renderer *renderer.Renderer
    frontend adapter.Frontend
}

func New(cfg *config.Config, fe adapter.Frontend) (*App, error) {
    term := terminal.New(cfg.Scrollback.Lines)
    met, _ := renderer.LoadMetrics(cfg.Font.Size)
    rend := renderer.New(met)
    p, _ := pty.Open(pty.Config{
        Command: resolveShell(cfg),
        Cols:    uint16(cfg.Window.Width / met.CellWidth),
        Rows:    uint16(cfg.Window.Height / met.CellHeight),
    })
    sess := vte.NewSession(p, term)
    sess.Notify = func() { fe.(interface{ RequestRedraw() }).RequestRedraw() }

    // Wire frontend callbacks
    fe.OnKeyInput(func(e adapter.KeyEvent) { p.Write(e.Data) })
    fe.OnResize(func(e adapter.ResizeEvent) {
        term.Resize(int(e.Cols), int(e.Rows))
        p.Resize(e.Cols, e.Rows)
        rend.Resize(int(e.Cols), int(e.Rows))
    })
    return &App{cfg: cfg, pty: p, session: sess,
                terminal: term, renderer: rend, frontend: fe}, nil
}

func (a *App) Run() error {
    go func() {
        if err := a.session.Run(); err != nil {
            logger.Get().Info("session ended", "err", err)
        }
        a.frontend.Close()
    }()
    go func() {
        a.pty.Wait()
        a.frontend.Close()
    }()
    return a.frontend.Run()
}
```

#### Phase 5 acceptance criteria
- [ ] `go run ./cmd/termizard` opens a working terminal window
- [ ] Shell spawns, prompt appears, typing works
- [ ] `vim /etc/hosts` → opens, edits, `:q` exits cleanly
- [ ] `htop` → colors, borders, resize works
- [ ] Closing window → process exits cleanly (no zombie PTY)
- [ ] Config file loaded from XDG path; missing file → defaults used

---

### Phase 6 — Polish & MVP Completeness
**Effort:** 2–3 days  
**Before cutting MVP tag**

| Feature | Detail |
|---|---|
| Copy | Mouse selection → copy to clipboard on release |
| Paste | Ctrl+Shift+V → paste from clipboard (bracketed paste: `\x1b[?2004h`) |
| Scrollback navigation | Ctrl+Shift+Up/Down, PgUp/PgDn to scroll view |
| Window title | `OSC 0;title ST` → `SetTitle` in gogpu |
| Cursor shapes | Block / beam / underline; blinking via timer goroutine |
| Bold font | Separate bold face or synthetic bold (brighten fg) |
| Mouse scroll | Wheel → `\x1b[65;...M` or `\x1b[A` / `\x1b[B` |
| Wide chars | CJK double-width: skip col+1, use Cell.Width=2 |
| True color | `\x1b[38;2;R;G;Bm` confirmed working in neovim |
| Performance | Verify < 5ms frame time at 80×24, < 1% CPU at idle |

---

## 4. Key Interfaces Summary

```go
// adapter.Frontend (boundary between app and window)
type Frontend interface {
    Run() error
    OnKeyInput(fn func(KeyEvent))
    OnResize(fn func(ResizeEvent))
    Close() error
}

// vte.Performer (boundary between parser and terminal model)
type Performer interface {
    Print(r rune)
    Execute(b byte)
    EscDispatch(intermediates []byte, final byte)
    CSI(params [][]uint16, intermediates []byte, ignore bool, final rune)
    OSC(params [][]byte, bellTerminated bool)
    DCS(params [][]uint16, intermediates []byte, ignore bool, final rune)
    DCSPut(b byte)
    DCSUnhook()
}

// renderer.Source (what the renderer reads from Terminal)
type Source interface {
    RLock(); RUnlock()
    Cols() int; Rows() int
    Cell(row, col int) terminal.Cell
    IsDirty(row int) bool
    ClearDirty()
    CursorPos() (col, row int)
    CursorVisible() bool
}
```

---

## 5. Risks & Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| gogpu API instability | Medium | Pin to v0.40.0; isolate in `gogpu_frontend` only |
| Wide char (CJK) rendering | Medium | Track `Cell.Width`, skip col+1 on print, test with `echo 中文` |
| macOS keyboard event routing (multi-window) | High (known issue) | Vendor patch: include `WindowID` in key events (already done in memory) |
| Font rasterization quality | Low (MVP uses bitmap) | Swap to opentype after MVP; renderer interface hides the impl |
| Performance regression | Medium | Dirty-row tracking + benchmark gate in CI: `go test -bench` |
| PTY on macOS draining (cmd.Wait blocks) | Medium (known) | Drain goroutine before Wait, already in integration test |

---

## 6. Milestones & Timeline

```
Week 1    Phase 2 — Terminal model (Grid, SGR, cursor, scrollback)
          Gate: vim + htop render correctly without window

Week 2    Phase 3 — CPU renderer (atlas, dirty-row Frame())
          Gate: Frame() PNG matches expected fixture

Week 3    Phase 4 — gogpu frontend (window, texture, keymap)
          Gate: type in shell, arrow keys work in vim

Week 4    Phase 5 — App wiring + config
          Gate: go run ./cmd/termizard opens working terminal

Week 5    Phase 6 — copy/paste, scrollback, cursor shapes, wide chars
          Gate: all MVP checklist items pass

Week 6    Buffer: performance profiling, edge cases, CI hardening
```

---

## 7. MVP Checklist

### Core terminal
- [ ] Spawn $SHELL with correct size
- [ ] SGR: reset, bold, dim, italic, underline, blink, inverse, invisible, strikethrough
- [ ] SGR colors: default, ANSI-16, 256-color, 24-bit true color
- [ ] Cursor: CUP (H/f), CUU/CUD/CUF/CUB, CNL/CPL, CHA, DECSC/DECRC
- [ ] Erase: ED (0/1/2/3), EL (0/1/2), ECH, DCH, ICH
- [ ] Scroll: DECSTBM, SU/SD
- [ ] Alt screen: SMCUP/RMCUP (?1049h/?1049l)
- [ ] Cursor visibility: ?25h/?25l
- [ ] App cursor keys mode: ?1h/?1l (DECCKM)
- [ ] Window title: OSC 0/2
- [ ] Bell: OSC 7? + Execute 0x07
- [ ] Wide chars: CJK double-width

### Input
- [ ] Printable ASCII + Unicode input
- [ ] Arrow keys, Home/End, PgUp/PgDn, Insert, Delete
- [ ] F1–F12
- [ ] Ctrl+letter (Ctrl+C, Ctrl+D, etc.)
- [ ] Alt+letter (sends `\x1b` prefix)
- [ ] Shift+F-key variants
- [ ] Bracketed paste (?2004h)
- [ ] Mouse scroll (wheel → CUU/CUD or mouse protocol)

### Window & UX
- [ ] Resize → SIGWINCH + grid reflow
- [ ] Copy on mouse selection (Ctrl+Shift+C)
- [ ] Paste (Ctrl+Shift+V)
- [ ] Scrollback navigation (Ctrl+Shift+Up/Down)
- [ ] Window title from OSC
- [ ] Cursor shapes: block / beam / underline
- [ ] Config file (TOML) loaded from XDG path

### Non-goals for MVP
- Tabs / splits
- Ligature shaping
- Sixel / kitty image protocol
- Search
- Vi mode
- IME / input method
- Two-pass GPU pipeline (post-MVP optimisation)

---

## 8. Build & Run Commands

```bash
# Build
go build ./...

# Run (after Phase 5)
go run ./cmd/termizard

# All tests
go test -race ./... -count=1 -timeout 60s

# Terminal model only
go test ./internal/core/terminal/... -v

# Benchmark renderer
go test ./internal/renderer/... -bench=. -benchmem

# Generate coverage
go test ./... -coverprofile=cover.out && go tool cover -html=cover.out
```

---

## 9. File Map (target state after MVP)

```
termizard/
├── cmd/termizard/
│   └── main.go                     # parse flags, load config, run app
├── internal/
│   ├── app/
│   │   └── app.go                  # App struct: wires all layers
│   ├── adapter/
│   │   └── frontend.go             # Frontend interface (already done)
│   ├── core/
│   │   ├── pty/                    # ✅ done
│   │   ├── vte/                    # ✅ done
│   │   └── terminal/               # Phase 2
│   │       ├── cell.go             # Cell, Color, Attrs
│   │       ├── grid.go             # Grid (flat []Cell + dirty bits)
│   │       ├── scrollback.go       # ring buffer
│   │       ├── screen.go           # Screen (primary+alt, cursor, scroll region)
│   │       ├── sgr.go              # SGR decoder
│   │       ├── terminal.go         # Terminal: implements vte.Performer
│   │       └── terminal_test.go
│   ├── renderer/
│   │   ├── atlas.go                # glyph atlas (shelf packing)
│   │   ├── font.go                 # FontMetrics loader
│   │   ├── renderer.go             # Frame() → *image.RGBA
│   │   └── renderer_test.go
│   ├── frontend/
│   │   ├── gogpu_frontend/         # Phase 4
│   │   │   ├── frontend.go
│   │   │   └── keymap.go
│   │   └── mock_frontend/          # ✅ done
│   └── util/
│       ├── logger/                 # ✅ done
│       └── config/                 # Phase 5
│           └── config.go
├── docs/
│   └── plans/
│       ├── MVP-PLAN.md             # this file
│       └── phase-1-core-pty-vte.md
├── go.mod
└── Makefile
```
