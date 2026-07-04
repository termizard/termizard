# Termizard — MVP Implementation Plan

**Stack:** Go 1.26 · gogpu v0.40+ (windowing/GPU) · gg (2D rendering) · ggcanvas (bridge) · creack/pty · Paul Williams VTE parser  
**References:** kitty (keyboard protocol, colors), alacritty (simplicity, correctness), rio (config design, GPU pipeline)  
**Target:** macOS + Linux. Windows ConPTY stub already in place.

---

## 1. Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  cmd/termizard                                                   │
│  main() → config.Load() → app.New(cfg) → app.Run()              │
└───────────┬──────────────────────────────────────────────────────┘
            │
┌───────────▼──────────────┐     ┌──────────────────────────────┐
│  internal/app             │     │  internal/util/config        │
│  App: wires all layers    │◄────│  TOML loader + defaults      │
└──┬──────────┬─────────────┘     └──────────────────────────────┘
   │          │
   │  goroutine: PTY reader
   │          │
┌──▼──────┐  ┌▼─────────────────┐
│core/pty │  │core/vte           │
│PTY      │─►│Session → Parser   │
│interface│  │→ Performer        │
│POSIX /  │  └────────┬──────────┘
│ConPTY   │           │ Print/CSI/ESC/OSC calls
└─────────┘           │
              ┌────────▼──────────┐
              │core/terminal       │
              │Terminal (Performer │
              │impl):              │
              │ · Screen (Grid)    │
              │ · Scrollback       │
              │ · SGR decoder      │
              │ · Cursor state     │
              └────────┬───────────┘
                       │ dirty flag → RequestRedraw
                       │
              ┌────────▼───────────────────────────────────────────┐
              │  adapter.Frontend (interface)                       │
              │                                                     │
              │  ┌─────────────────────┐  ┌────────────────────┐  │
              │  │  gogpu_frontend     │  │  mock_frontend     │  │
              │  │  gogpu.App          │  │  headless/tests    │  │
              │  │  + ggcanvas.Canvas  │  └────────────────────┘  │
              │  │  + gg cell renderer │                           │
              │  └─────────────────────┘                           │
              └────────────────────────────────────────────────────┘
```

### Goroutine model

```
main goroutine         │ gogpu event loop (app.Run) — MUST stay on main thread
PTY reader goroutine   │ reads bytes → vte.Session → Terminal (mutex-locked)
                       │ → atomic dirty flag → app.RequestRedraw()
render callback        │ inside app.OnDraw: lock Terminal, draw cells with gg,
                       │ canvas.Render(dc.RenderTarget()) → GPU present
```

### Rendering pipeline (gg-based, no custom CPU renderer)

```
Terminal.Cell(row,col) → gg.Context.DrawRectangle (BG)
                       → gg.Context.DrawString     (glyph via MSDF/LCD)
                       → ggcanvas.Canvas.Render    → GPU surface
```

No custom glyph atlas. No `*image.RGBA` upload. gg handles font rasterization,
subpixel rendering, and HiDPI automatically. Same pattern as the gg+gogpu
integration example (`examples/gogpu_integration`).

---

## 2. Component Table

| Component | Package | Status | Responsibility |
|---|---|---|---|
| PTY | `internal/core/pty` | ✅ done | spawn shell, resize, R/W |
| VTE parser | `internal/core/vte` | ✅ done | byte stream → Performer callbacks |
| Terminal model | `internal/core/terminal` | ✅ done | screen grid, SGR, cursor, scrollback |
| Frontend interface | `internal/adapter` | ✅ done | boundary: key/resize events |
| Mock frontend | `internal/frontend/mock_frontend` | ✅ done | headless for tests |
| Logger | `internal/util/logger` | ✅ done | zero-cost slog wrapper |
| gogpu frontend | `internal/frontend/gogpu_frontend` | Phase 3 | window + gg rendering + keymap |
| App | `internal/app` | Phase 4 | wires all layers, goroutine lifecycle |
| Config | `internal/util/config` | Phase 4 | TOML loader, sane defaults |

---

## 3. Phases

### Phase 1 — PTY + VTE ✅ DONE
### Phase 2 — Terminal Model ✅ DONE

---

### Phase 3 — gogpu Frontend
**Package:** `internal/frontend/gogpu_frontend`  
**Effort:** 3–4 days  
**Deps:** `github.com/gogpu/gogpu`, `github.com/gogpu/gg`, `github.com/gogpu/gg/integration/ggcanvas`  
**Ref:** gogpu `examples/gpucontext_integration`, gg `examples/gogpu_integration`

#### 3.1 Frontend struct

```go
// frontend.go
package gogpu_frontend

import (
    "sync/atomic"

    "github.com/gogpu/gg"
    _ "github.com/gogpu/gg/gpu" // register GPU accelerator (MSDF text + SDF shapes)
    "github.com/gogpu/gg/integration/ggcanvas"
    "github.com/gogpu/gg/text"
    "github.com/gogpu/gogpu"
    "github.com/gogpu/gpucontext"

    "github.com/termizard/termizard/internal/adapter"
    "github.com/termizard/termizard/internal/core/terminal"
    "github.com/termizard/termizard/internal/util/config"
)

type GogpuFrontend struct {
    cfg      *config.Config
    term     *terminal.Terminal

    app    *gogpu.App
    canvas *ggcanvas.Canvas
    face   text.Face // monospace font face at cfg.Font.Size

    dirty    atomic.Bool
    keyFn    func(adapter.KeyEvent)
    resizeFn func(adapter.ResizeEvent)
}

var _ adapter.Frontend = (*GogpuFrontend)(nil)

func New(cfg *config.Config, term *terminal.Terminal) *GogpuFrontend {
    f := &GogpuFrontend{cfg: cfg, term: term}
    f.app = gogpu.NewApp(gogpu.DefaultConfig().
        WithTitle(cfg.Window.Title).
        WithSize(cfg.Window.Width, cfg.Window.Height).
        WithResizable(true).
        WithContinuousRender(false)) // event-driven: 0% CPU when idle
    return f
}
```

#### 3.2 Run and event wiring

```go
func (f *GogpuFrontend) Run() error {
    f.app.OnDraw(f.onDraw)

    ev := f.app.EventSource()
    ev.OnKeyPress(f.onKeyPress)
    ev.OnTextInput(f.onTextInput) // printable chars / IME
    ev.OnScroll(f.onScroll)
    ev.OnResize(f.onResized)

    return f.app.Run()
}

func (f *GogpuFrontend) Close() error {
    f.app.Quit()
    return nil
}

func (f *GogpuFrontend) OnKeyInput(fn func(adapter.KeyEvent)) { f.keyFn = fn }
func (f *GogpuFrontend) OnResize(fn func(adapter.ResizeEvent)) { f.resizeFn = fn }

// RequestRedraw is called from PTY goroutine when terminal state changes.
func (f *GogpuFrontend) RequestRedraw() {
    f.dirty.Store(true)
    f.app.RequestRedraw()
}
```

#### 3.3 Draw callback

```go
func (f *GogpuFrontend) onDraw(dc *gogpu.Context) {
    w, h := dc.Width(), dc.Height()
    if w <= 0 || h <= 0 {
        return
    }

    // Lazy-init canvas (needs GPU device, only available after first OnDraw).
    if f.canvas == nil {
        provider := f.app.GPUContextProvider()
        if provider == nil {
            return
        }
        f.canvas, _ = ggcanvas.New(provider, w, h)
        f.loadFont(dc.ScaleFactor())
    }

    // Handle resize.
    if cw, ch := f.canvas.Size(); cw != w || ch != h {
        _ = f.canvas.Resize(w, h)
    }

    _ = f.canvas.Draw(func(cc *gg.Context) {
        f.renderTerminal(cc, w, h)
    })
    _ = f.canvas.Render(dc.RenderTarget())

    f.dirty.Store(false)
}

func (f *GogpuFrontend) loadFont(scale float64) {
    // Try config font first, fall back to system monospace.
    src := loadFontSource(f.cfg.Font.Family)
    if src != nil {
        f.face = src.Face(f.cfg.Font.Size)
    }
}
```

#### 3.4 Cell renderer

```go
// renderer.go
func (f *GogpuFrontend) renderTerminal(cc *gg.Context, w, h int) {
    f.term.RLock()
    defer f.term.RUnlock()

    cols, rows := f.term.Cols(), f.term.Rows()
    if cols == 0 || rows == 0 {
        return
    }
    cw := float64(w) / float64(cols)
    ch := float64(h) / float64(rows)

    // Background fill.
    bg := parseColor(f.cfg.Colors.Background)
    cc.SetRGBA(bg.R, bg.G, bg.B, 1)
    cc.DrawRectangle(0, 0, float64(w), float64(h))
    cc.Fill()

    curCol, curRow := f.term.CursorPos()

    for row := 0; row < rows; row++ {
        for col := 0; col < cols; col++ {
            cell := f.term.Cell(row, col)
            x := float64(col) * cw
            y := float64(row) * ch

            // Cell background (skip if default).
            cellBG := resolveColor(cell.BG, f.cfg, false)
            if cell.Attrs&terminal.AttrInverse != 0 {
                cellBG = resolveColor(cell.FG, f.cfg, true)
            }
            if !isDefaultColor(cell.BG) || cell.Attrs&terminal.AttrInverse != 0 {
                cc.SetRGBA(cellBG.R, cellBG.G, cellBG.B, 1)
                cc.DrawRectangle(x, y, cw*float64(cell.Width), ch)
                cc.Fill()
            }

            // Glyph.
            if cell.Char != 0 && cell.Char != ' ' && f.face != nil {
                fg := resolveColor(cell.FG, f.cfg, true)
                if cell.Attrs&terminal.AttrInverse != 0 {
                    fg = resolveColor(cell.BG, f.cfg, false)
                }
                if cell.Attrs&terminal.AttrDim != 0 {
                    fg.A = 0.5
                }
                cc.SetRGBA(fg.R, fg.G, fg.B, fg.A)
                cc.SetFont(f.face)
                // Baseline = top of cell + ascent.
                cc.DrawString(string(cell.Char), x, y+f.ascent)
            }
        }
    }

    // Cursor.
    if f.term.CursorVisible() {
        cx := float64(curCol) * cw
        cy := float64(curRow) * ch
        cc.SetRGBA(cursorR, cursorG, cursorB, 0.85)
        switch f.cfg.Cursor.Shape {
        case "beam":
            cc.DrawRectangle(cx, cy, 2, ch)
        case "underline":
            cc.DrawRectangle(cx, cy+ch-2, cw, 2)
        default: // block
            cc.DrawRectangle(cx, cy, cw, ch)
        }
        cc.Fill()
    }
}
```

#### 3.5 Key mapping (kitty-style)

```go
// keymap.go
// Map gpucontext.Key + Modifiers → PTY byte sequence.
// Mirrors kitty's terminfo entries; respects Terminal.AppCursorKeys().

func keyToBytes(key gpucontext.Key, mods gpucontext.Modifiers, appCursor bool) []byte {
    if mods.Control() {
        if b, ok := ctrlKeys[key]; ok {
            return []byte{b}
        }
    }
    if appCursor {
        if seq, ok := appCursorKeys[key]; ok {
            return []byte(seq)
        }
    }
    if seq, ok := specialKeys[key]; ok {
        return []byte(seq)
    }
    return nil
}

var specialKeys = map[gpucontext.Key]string{
    gpucontext.KeyUp:       "\x1b[A",
    gpucontext.KeyDown:     "\x1b[B",
    gpucontext.KeyRight:    "\x1b[C",
    gpucontext.KeyLeft:     "\x1b[D",
    gpucontext.KeyHome:     "\x1b[H",
    gpucontext.KeyEnd:      "\x1b[F",
    gpucontext.KeyPageUp:   "\x1b[5~",
    gpucontext.KeyPageDown: "\x1b[6~",
    gpucontext.KeyInsert:   "\x1b[2~",
    gpucontext.KeyDelete:   "\x1b[3~",
    gpucontext.KeyF1:       "\x1bOP",
    gpucontext.KeyF2:       "\x1bOQ",
    gpucontext.KeyF3:       "\x1bOR",
    gpucontext.KeyF4:       "\x1bOS",
    gpucontext.KeyF5:       "\x1b[15~",
    gpucontext.KeyF6:       "\x1b[17~",
    gpucontext.KeyF7:       "\x1b[18~",
    gpucontext.KeyF8:       "\x1b[19~",
    gpucontext.KeyF9:       "\x1b[20~",
    gpucontext.KeyF10:      "\x1b[21~",
    gpucontext.KeyF11:      "\x1b[23~",
    gpucontext.KeyF12:      "\x1b[24~",
    gpucontext.KeyEscape:   "\x1b",
    gpucontext.KeyBackspace: "\x7f",
    gpucontext.KeyTab:      "\x09",
    gpucontext.KeyEnter:    "\r",
}

var appCursorKeys = map[gpucontext.Key]string{
    gpucontext.KeyUp:    "\x1bOA",
    gpucontext.KeyDown:  "\x1bOB",
    gpucontext.KeyRight: "\x1bOC",
    gpucontext.KeyLeft:  "\x1bOD",
}

var ctrlKeys = map[gpucontext.Key]byte{
    // Ctrl+A..Z → 0x01..0x1A
    // Mapped dynamically from key rune.
}
```

#### 3.6 Resize

```go
func (f *GogpuFrontend) onResized(w, h int) {
    if f.resizeFn == nil || f.face == nil {
        return
    }
    metrics := f.face.Metrics()
    cw := int(metrics.Advance("M"))  // monospace: all same width
    ch := int(metrics.LineHeight())
    if cw <= 0 || ch <= 0 {
        return
    }
    cols := uint16(w / cw)
    rows := uint16(h / ch)
    if cols < 1 { cols = 1 }
    if rows < 1 { rows = 1 }
    f.resizeFn(adapter.ResizeEvent{Cols: cols, Rows: rows})
}
```

#### Phase 3 acceptance criteria
- [ ] Window opens with correct background color from config
- [ ] Shell prompt renders with correct colors
- [ ] Typing characters appears on screen in real time
- [ ] Arrow keys work in `vim`
- [ ] `htop` renders: alt-screen, colors, borders
- [ ] Resize → shell reflows correctly
- [ ] Idle CPU < 1% (event-driven: `ContinuousRender(false)`)
- [ ] HiDPI (Retina): no blurry text, correct scale

---

### Phase 4 — App Wiring + Config
**Packages:** `internal/app`, `internal/util/config`  
**Effort:** 2 days

#### 4.1 Config (TOML, Rio-inspired)

```toml
# ~/.config/termizard/config.toml

[window]
title   = "termizard"
width   = 1200
height  = 800
opacity = 1.0

[font]
family = ""        # "" = system monospace (searched on macOS/Linux)
size   = 14.0

[colors]
background = "#1a1a2e"
foreground = "#e0e0e0"
cursor     = "#f38ba8"
selection  = "#44475a"

  [colors.ansi]
  # 0-7: normal, 8-15: bright (optional override)
  black         = "#000000"
  red           = "#ff5555"
  green         = "#50fa7b"
  yellow        = "#f1fa8c"
  blue          = "#bd93f9"
  magenta       = "#ff79c6"
  cyan          = "#8be9fd"
  white         = "#bfbfbf"
  bright_black  = "#4d4d4d"
  bright_red    = "#ff6e67"
  bright_green  = "#5af78e"
  bright_yellow = "#f4f99d"
  bright_blue   = "#caa9fa"
  bright_magenta = "#ff92d0"
  bright_cyan   = "#9aedfe"
  bright_white  = "#e6e6e6"

[shell]
program = ""   # "" = $SHELL
args    = []

[scrollback]
lines = 10000

[cursor]
shape = "block"   # block | beam | underline
blink = false

# Key bindings — same style as Rio
[[keybindings]]
key    = "v"
mods   = ["Super", "Shift"]
action = "Paste"

[[keybindings]]
key    = "c"
mods   = ["Super", "Shift"]
action = "Copy"

[[keybindings]]
key    = "Up"
mods   = ["Super", "Shift"]
action = "ScrollUp"

[[keybindings]]
key    = "Down"
mods   = ["Super", "Shift"]
action = "ScrollDown"
```

```go
// config.go
type Config struct {
    Window     WindowConfig
    Font       FontConfig
    Colors     ColorConfig
    Shell      ShellConfig
    Scrollback ScrollbackConfig
    Cursor     CursorConfig
    Keybindings []Keybinding
}

type WindowConfig struct {
    Title   string
    Width   int
    Height  int
    Opacity float64
}

type FontConfig struct {
    Family string
    Size   float64
}

type ColorConfig struct {
    Background string
    Foreground string
    Cursor     string
    Selection  string
    ANSI       ANSIColors `toml:"ansi"`
}

type Keybinding struct {
    Key    string
    Mods   []string
    Action string // "Paste" | "Copy" | "ScrollUp" | "ScrollDown"
}

func Defaults() *Config { ... }
func Load(path string) (*Config, error) { ... } // BurntSushi/toml
func DefaultPath() string { ... }               // XDG_CONFIG_HOME/termizard/config.toml
```

#### 4.2 App

```go
// app.go
type App struct {
    cfg      *config.Config
    pty      pty.PTY
    session  *vte.Session
    terminal *terminal.Terminal
    frontend adapter.Frontend
}

type frontendWithRedraw interface {
    adapter.Frontend
    RequestRedraw()
}

func New(cfg *config.Config) (*App, error) {
    term := terminal.New(
        cfg.Window.Width/estimateCellWidth(cfg),
        cfg.Window.Height/estimateCellHeight(cfg),
        cfg.Scrollback.Lines,
    )
    fe := gogpu_frontend.New(cfg, term)

    p, err := pty.Open(pty.Config{
        Command: resolveShell(cfg),
        Cols:    uint16(term.Cols()),
        Rows:    uint16(term.Rows()),
    })
    if err != nil {
        return nil, err
    }

    sess := vte.NewSession(p, term)
    if rd, ok := fe.(frontendWithRedraw); ok {
        sess.Notify = rd.RequestRedraw
    }

    fe.OnKeyInput(func(e adapter.KeyEvent) {
        p.Write(e.Data)
    })
    fe.OnResize(func(e adapter.ResizeEvent) {
        term.Resize(int(e.Cols), int(e.Rows))
        p.Resize(e.Cols, e.Rows)
    })

    return &App{cfg: cfg, pty: p, session: sess, terminal: term, frontend: fe}, nil
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

#### Phase 4 acceptance criteria
- [ ] `go run ./cmd/termizard` opens a working terminal window
- [ ] Config loaded from `~/.config/termizard/config.toml`; missing → defaults
- [ ] Custom colors from config applied correctly
- [ ] Shell exits cleanly → window closes (no zombie PTY)
- [ ] `vim /etc/hosts` → opens, edits, `:q` exits cleanly

---

### Phase 5 — Polish & MVP Completeness
**Effort:** 2–3 days

| Feature | Detail |
|---|---|
| Copy | Mouse selection → clipboard on release |
| Paste | `Ctrl+Shift+V` / `Super+Shift+V` → bracketed paste (`\x1b[?2004h`) |
| Scrollback nav | `Ctrl+Shift+Up/Down`, PgUp/PgDn scroll view (not PTY) |
| Window title | `OSC 0/2` → `app.SetTitle()` |
| Cursor blink | Timer goroutine → `RequestRedraw` at 500ms intervals when blink=true |
| Bold font | Load separate bold face from `FontSource`; fall back to synthetic bold |
| Mouse scroll | Wheel → `\x1b[65;...M` (mouse protocol) or `\x1b[A`/`\x1b[B` |
| Wide chars | CJK: `cell.Width==2` → skip col+1, draw at double cell width |
| True color | `\x1b[38;2;R;G;Bm` confirmed working in neovim |
| Performance | Verify < 5ms draw time per frame, < 1% CPU at idle |

---

## 4. Key Interfaces

```go
// adapter.Frontend — boundary between app and window system
type Frontend interface {
    Run() error
    OnKeyInput(fn func(KeyEvent))
    OnResize(fn func(ResizeEvent))
    Close() error
}

// Optional: implemented by gogpu_frontend for PTY→GPU redraw notification
type FrontendWithRedraw interface {
    Frontend
    RequestRedraw()
}

// vte.Performer — boundary between VTE parser and terminal model (already defined)
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
```

---

## 5. gogpu + gg Integration Notes

### Module paths (pinned)
```
github.com/gogpu/gogpu          — windowing, input, GPU lifecycle
github.com/gogpu/gg             — 2D drawing (shapes, text, paths)
github.com/gogpu/gg/integration/ggcanvas — CPU→GPU bridge (lazy texture upload)
github.com/gogpu/gpucontext     — shared GPU context types (Device, Key, etc.)
```

### The rendering pattern

```go
// Inside OnDraw callback:
canvas.Draw(func(cc *gg.Context) {
    // All drawing here — pure gg API, no GPU calls directly
    cc.SetRGBA(...)
    cc.DrawRectangle(...)
    cc.DrawString(...)
})
canvas.Render(dc.RenderTarget()) // one call uploads + presents
```

### Event-driven rendering (0% idle CPU)

```go
app := gogpu.NewApp(gogpu.DefaultConfig().WithContinuousRender(false))

// PTY goroutine signals new data:
func (f *GogpuFrontend) RequestRedraw() {
    f.app.RequestRedraw() // thread-safe, coalesced
}

// Inside OnDraw — do NOT call RequestRedraw unconditionally;
// only when dirty to avoid busy-looping:
f.dirty.Store(false)
// If more PTY data arrived during this frame, RequestRedraw will
// be called again by the PTY goroutine.
```

### Multi-window (Phase 5+, no tabs)

```go
// Create additional window from OnUpdate (after renderer init):
app.OnUpdate(func(dt float64) {
    if needNewWindow {
        w2, _ := app.NewWindow(gogpu.DefaultConfig().WithTitle("termizard 2"))
        w2.SetOnDraw(func(dc *gogpu.Context) { ... })
        w2.SetOnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) { ... })
    }
})
```

### Font loading

```go
// text.NewFontSourceFromFile → text.FontSource.Face(size) → text.Face
// For MVP: search system paths (macOS, Linux).
// Config font.family overrides search.
candidates := []string{
    // macOS monospace
    "/System/Library/Fonts/Menlo.ttc",
    "/Library/Fonts/Courier New.ttf",
    // Linux
    "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
    "/usr/share/fonts/TTF/JetBrainsMono-Regular.ttf",
}
```

---

## 6. Risks & Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| gogpu/gg API churn | Medium | Pin to a tagged release; isolate all gogpu/gg calls inside `gogpu_frontend` |
| Cell-width-based column sizing imprecise | Medium | Use `face.Metrics().Advance("M")` for monospace cell width; test with `echo 中文` |
| PTY resize race with terminal render | Low | `terminal.Resize` holds write lock; renderer holds read lock — safe |
| Font not found on CI | High | Fall back to `golang.org/x/image/font/basicfont` for headless; skip text in mock |
| macOS keyboard multi-window routing | Medium | Use `Window.SetOnKeyPress` per-window, not app-level events |
| PTY drain on macOS (cmd.Wait blocks) | Medium | Drain goroutine before Wait; already tested in integration test |
| Idle CPU > 1% | Low | `ContinuousRender(false)` + dirty-gated `RequestRedraw` — verified in gg examples |

---

## 7. MVP Checklist

### Core terminal
- [x] Spawn $SHELL with correct size
- [x] SGR: reset, bold, dim, italic, underline, blink, inverse, invisible, strikethrough
- [x] SGR colors: default, ANSI-16, 256-color, 24-bit true color
- [x] Cursor: CUP, CUU/CUD/CUF/CUB, CNL/CPL, CHA, DECSC/DECRC
- [x] Erase: ED, EL, ECH, DCH, ICH
- [x] Scroll: DECSTBM, SU/SD
- [x] Alt screen: ?1049h/?1049l
- [x] Cursor visibility: ?25h/?25l
- [x] App cursor keys: ?1h/?1l (DECCKM)
- [x] Window title: OSC 0/2
- [x] Bell: 0x07
- [x] Wide chars: CJK double-width

### Input
- [ ] Printable ASCII + Unicode (via `OnTextInput`)
- [ ] Arrow keys, Home/End, PgUp/PgDn, Insert, Delete
- [ ] F1–F12
- [ ] Ctrl+letter (Ctrl+C, Ctrl+D, etc.)
- [ ] Alt+letter (`\x1b` prefix)
- [ ] Shift+F-key variants
- [ ] Bracketed paste (?2004h)
- [ ] Mouse scroll

### Window & UX
- [ ] Window opens, correct size, correct background
- [ ] Resize → SIGWINCH + grid reflow
- [ ] Copy (mouse selection)
- [ ] Paste (keybinding from config)
- [ ] Scrollback navigation
- [ ] Window title from OSC
- [ ] Cursor shapes: block / beam / underline
- [ ] Config file (TOML) from XDG path
- [ ] Idle CPU < 1%

### Non-goals for MVP
- Tabs (use OS multi-window instead)
- Ligature shaping
- Sixel / kitty image protocol
- Search
- Vi mode
- IME / input method (basic `OnTextInput` only)
- Two-pass GPU background quads (post-MVP optimization)

---

## 8. Milestones

```
Week 1 (current)  Phase 3 — gogpu frontend (window, gg renderer, keymap)
                  Gate: shell prompt visible, typing + arrow keys work

Week 2            Phase 4 — App wiring + config (TOML, Rio-style)
                  Gate: go run ./cmd/termizard opens working terminal

Week 3            Phase 5 — copy/paste, scrollback, cursor shapes
                  Gate: all MVP checklist items pass

Week 4            Buffer: profiling, edge cases, CI hardening
```

---

## 9. File Map (target state after MVP)

```
termizard/
├── cmd/termizard/
│   └── main.go                     # parse flags, load config, New+Run
├── internal/
│   ├── app/
│   │   └── app.go                  # App: wires PTY+VTE+Terminal+Frontend
│   ├── adapter/
│   │   └── frontend.go             # Frontend interface ✅
│   ├── core/
│   │   ├── pty/                    ✅ done
│   │   ├── vte/                    ✅ done
│   │   └── terminal/               ✅ done
│   ├── frontend/
│   │   ├── gogpu_frontend/         # Phase 3
│   │   │   ├── frontend.go         # gogpu App setup, event wiring
│   │   │   ├── renderer.go         # gg cell rendering (colors, glyphs, cursor)
│   │   │   └── keymap.go           # Key+Mods → PTY escape sequence
│   │   └── mock_frontend/          ✅ done
│   └── util/
│       ├── logger/                 ✅ done
│       └── config/                 # Phase 4
│           └── config.go           # TOML loader + defaults
├── docs/
│   └── plans/
│       ├── MVP-PLAN.md
│       └── phase-1-core-pty-vte.md
├── go.mod
└── Makefile
```

---

## 10. Build & Run

```bash
# Install deps (after go.mod updated)
go get github.com/gogpu/gogpu@latest
go get github.com/gogpu/gg@latest

# Build
go build ./...

# Run (Phase 4+)
go run ./cmd/termizard

# Tests
go test -race ./... -count=1 -timeout 60s

# Terminal model
go test ./internal/core/terminal/... -v

# Lint
golangci-lint run ./...
```
