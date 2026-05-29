# ADR-001: GPU-Accelerated Terminal Emulator in Go
## Architecture Decision Record

```
Status:    ACCEPTED
Date:      2026-05-26
Authors:   —
Reviewers: —
Version:   1.0
```

---

## Table of Contents

1. [Context & Problem Statement](#1-context--problem-statement)
2. [Rio as Primary Reference](#2-rio-as-primary-reference)
3. [Rio vs Alacritty — Performance Reality](#3-rio-vs-alacritty--performance-reality)
4. [Decision 1 — GPU Framework](#4-decision-1--gpu-framework)
5. [Decision 2 — Module Structure](#5-decision-2--module-structure)
6. [Decision 3 — Sugar Renderer](#6-decision-3--sugar-renderer)
7. [Decision 4 — PTY & Event Loop](#7-decision-4--pty--event-loop)
8. [Decision 5 — VTE Parser](#8-decision-5--vte-parser)
9. [Target Metrics](#9-target-metrics)
10. [Dependencies](#10-dependencies)
11. [Implementation Phases](#11-implementation-phases)
12. [Rejected Alternatives](#12-rejected-alternatives)
13. [Risks & Mitigations](#13-risks--mitigations)
14. [References](#14-references)

---

## 1. Context & Problem Statement

We are building a GPU-accelerated terminal emulator in Go.  
The goal is not to write a "basic terminal" — it is to implement the architectural ideas
that make **Rio Terminal** fast and cross-platform, but expressed natively in Go.

**Why Go instead of Rust?**  
Our team's existing codebase, tooling, and hiring are Go-first. Rust's compile times and
learning curve are real costs. Go's goroutine model maps naturally onto the concurrency
structure a terminal needs (PTY I/O, render loop, event loop as independent goroutines).
The performance gap between Go and Rust for this workload — dominated by GPU time and
kernel I/O, not CPU computation — is acceptable.

**Why not wrap an existing terminal?**  
Embedding libvte, xterm.js, or linking against Alacritty would eliminate the architectural
flexibility we want: custom Sugar rendering, gogpu integration, future WASM target.

---

## 2. Rio as Primary Reference

Rio Terminal (https://github.com/raphamorim/rio, v0.4.6 as of 2026) is our direct
architectural blueprint. We mirror its workspace decomposition 1:1, translating each Rust
crate into a Go package:

### Rio Cargo Workspace → Go Package Mapping

```
Rio crate             │ Role                              │ Go package
──────────────────────┼───────────────────────────────────┼──────────────────────
teletypewriter        │ PTY creation, fork/exec, resize   │ internal/pty
copa                  │ VTE ANSI parser (fork of vte)     │ go-vte library + internal/vte
corcovado             │ I/O event loop (fork of mio)      │ internal/eventloop  (goroutines)
rio-backend           │ terminal model, grid, selection   │ internal/terminal
rio-grapheme-width    │ Unicode grapheme cluster width    │ internal/grapheme
rio-notifier          │ file/config change notifications  │ internal/watcher
sugarloaf             │ GPU renderer (wgpu + fonts)       │ internal/renderer
rio-window            │ windowing (fork of winit)         │ internal/window     (gogpu)
frontends/rioterm     │ app binary, tabs, splits, config  │ cmd/goterm + internal/app
```

### Rio's Key Architectural Decisions We Adopt

**Sugar rendering model** — the central idea of sugarloaf.  
Every cell is a `Sugar{char, fg, bg, attrs}`. Identical runs of sugars are batched into
a `SugarRun`. Lines that did not change are detected by hash and skipped entirely.
Result: only dirty regions touch the GPU each frame.

**Two-pass GPU rendering**  
Pass 1 draws solid background rectangles for all cells.  
Pass 2 draws glyph quads from a texture atlas on top.  
Both passes share a single wgpu render pass (as of sugarloaf rewrite in v0.2).

**Redux-like state machine for screen updates**  
Rio tracks which lines are "dirty" after VTE dispatch. Only dirty lines are re-uploaded
to the vertex buffer. Clean lines reuse the previous frame's geometry.

**simdutf for UTF-8 decoding** (Rio v0.4 added `simdutf = "0.7.0"`)  
PTY output is raw bytes. Fast UTF-8 validation/decoding matters at high throughput.
In Go we use `unicode/utf8` + manual hot-path optimisation; for the future consider
cgo-linking simdutf if benchmarks show it bottlenecks.

**wgpu backends** — Metal (macOS), Vulkan (Linux/Win), DX12 (Win), GLES (embedded).  
We replicate this via `gogpu/gogpu` which supports the same backend matrix through its
own pure-Go wgpu implementation and an optional wgpu-native Rust backend.

---

## 3. Rio vs Alacritty — Performance Reality

Understanding where Rio stands relative to Alacritty is essential for setting
realistic targets. The picture is more nuanced than marketing copy suggests.

### 3.1 What Alacritty Optimises For

Alacritty was announced in 2017 as "the fastest terminal emulator in existence" and
benchmarked primarily on **raw PTY throughput** — how many bytes per second it can
read from the PTY and flush to screen. It achieves this by being deliberately minimal:
no tabs, no splits, no ligatures, no images. OpenGL rendering, direct vertex upload
every frame.

### 3.2 What Rio Optimises For

Rio's performance thesis is different: **do less GPU work per frame**, not just more
bytes/sec. The Sugarloaf de-duplication algorithm means that when 90% of the screen is
unchanged (normal interactive use), Rio touches almost nothing on the GPU.

Published numbers from Rio v0.0.15 release notes (100,000 chars with repetition):

| Metric                          | Before          | After (v0.0.15) | Gain  |
|---------------------------------|-----------------|-----------------|-------|
| `sugarloaf.stack()` call        | ~253.5 µs       | ~51.5 µs        | 5×    |
| First render, normal screen     | ~6 ms avg       | ~2 ms avg       | 3×    |
| First render, large (≥136 cols) | ~36 ms avg      | ~8 ms avg       | 4.5×  |

These are **internal** benchmarks, not comparisons against Alacritty. No published
head-to-head vtebench numbers between Rio and Alacritty exist as of May 2026.

### 3.3 vtebench Throughput Landscape (community runs, 2024–2025)

```
PTY bytes/sec throughput (vtebench, higher = better):

  foot (Wayland-native, C)   ████████████████████  best in class on Linux
  Alacritty (OpenGL)         ████████████████░░░░  very high
  Kitty (OpenGL)             ███████████████░░░░░  very high
  Rio (wgpu)                 ████████████░░░░░░░░  good — trades raw for smart
  WezTerm (wgpu)             ██████████░░░░░░░░░░  moderate
  iTerm2                     █████░░░░░░░░░░░░░░░  low
```

Rio's lower throughput than Alacritty on vtebench is intentional: vtebench floods the
terminal with data as fast as possible — the opposite of interactive use. Rio's
de-duplication pays off in the common case; it does extra work in the pathological case.

### 3.4 Input Latency (macOS, "Is It Snappy?" tool)

Counterintuitively, Alacritty does **not** win on input latency:

| Terminal             | Input latency  |
|----------------------|---------------|
| Kitty (unlimited)    | 29 ms ← best  |
| Kitty (vsync)        | 38 ms         |
| Terminal.app         | 45 ms         |
| WezTerm              | 46 ms         |
| **Alacritty**        | **58 ms**     |
| iTerm2 (GPU)         | 63 ms         |

Source: https://dev.to/lkhrs/measuring-terminal-latency-26m7

Rio's latency sits in the Alacritty-WezTerm band. The leader (Kitty) achieves its
latency by optionally bypassing vsync entirely.

### 3.5 Our Take

| Claim                              | Reality                                          |
|------------------------------------|--------------------------------------------------|
| "Rio is faster than Alacritty"     | Not on raw throughput. Smarter on GPU utilisation |
| "Alacritty is the fastest"         | Only on vtebench flood. Not on input latency.    |
| "wgpu > OpenGL"                    | More portable and modern; throughput comparable  |
| "Rio's Sugar model is a win"       | Yes — for interactive use, not for `cat bigfile` |

**Our target**: match Alacritty's throughput floor, match Rio's GPU efficiency,
target Kitty-class latency (<40 ms) via gogpu's event-driven rendering model.

---

## 4. Decision 1 — GPU Framework

### Problem

We need: GPU rendering (Metal/Vulkan/DX12), a native window, keyboard/mouse input,
HiDPI, clipboard — all without CGO (for portable static builds).

### Options Evaluated

#### A. `gogpu/gogpu` + `gogpu/wgpu`  ✅ CHOSEN

Pure-Go GPU framework. Dual backend: Pure Go wgpu (default) or wgpu-native Rust (`-tags rust`).

```go
app := gogpu.NewApp(gogpu.DefaultConfig().
    WithTitle("GoTerm").
    WithSize(1200, 800).
    WithContinuousRender(false))   // event-driven: 0% CPU when idle

app.OnDraw(func(dc *gogpu.Context) {
    renderer.Frame(dc.SurfaceView())
})
app.Run()
```

| Property          | Value                                              |
|-------------------|----------------------------------------------------|
| Metal (macOS)     | ✅ Pure Go Cocoa + goffi Objective-C runtime        |
| Vulkan            | ✅ X11 + Wayland                                   |
| DX12              | ✅ Win32 native                                    |
| CGO               | ❌ not required (`CGO_ENABLED=0`)                  |
| Window            | ✅ built-in (Win32/Cocoa/X11/Wayland)              |
| Input             | ✅ built-in (keyboard, mouse, clipboard, IME)      |
| HiDPI             | ✅ per-monitor DPI, logical/physical split         |
| Event-driven loop | ✅ three-state: idle / animating / continuous      |
| Rust backend opt  | ✅ `-tags rust` → wgpu-native (same as Rio's wgpu) |
| Status            | v0.25, 172 commits, Dec 2025 →                    |

The three-state rendering model directly solves the "terminal uses 100% CPU when idle"
problem that plagues continuous-render GPU terminals:

```
Idle (no PTY data, no keypress)  → blocks on OS events → 0% CPU
PTY data arrives or key pressed  → RequestRedraw() fires → one frame rendered
Animation (cursor blink, etc.)   → StartAnimation() token → vsync frames
```

This maps exactly to how Rio's `renderer.disable-unfocused-render` config option works,
but baked in at the framework level.

#### B. `go-webgpu/webgpu` (FFI → wgpu-native)  ❌

Low-level wgpu bindings. No window manager included. Would need GLFW separately.
Two libraries to coordinate, two dependency chains. Beta quality.

#### C. `cogentcore/webgpu` (CGO → wgpu-native)  ❌

Requires CGO. Breaks `CGO_ENABLED=0`. Complicates Docker, CI, cross-compilation.
Static binaries become impossible.

#### D. Pure OpenGL via `go-gl`  ❌

Deprecated on macOS since 10.14 (Mojave). No compute shaders. No WebGPU compatibility
path for future WASM target. Closes off Retro shader support (Rio has this as an opt-in
via wgpu feature flag).

### Decision

**Use `gogpu/gogpu`.**

It provides the complete platform layer (window + input + GPU) in one Zero-CGO package,
with the same wgpu-native backend that Rio uses available via build tag. WGSL shaders
written for Rio's sugarloaf are directly portable.

The escape hatch is clear: if a critical bug appears in the Pure Go backend on Apple
Silicon, switch to `-tags rust` which uses the same battle-tested wgpu-native binary
that Rio ships.

---

## 5. Decision 2 — Module Structure

We follow Rio's separation of concerns faithfully, replacing Rust crates with Go packages.

```
goterm/
│
├── cmd/
│   └── goterm/
│       └── main.go                 # entry point — wires everything together
│
├── internal/
│   │
│   ├── pty/                        # ← teletypewriter (Rio)
│   │   ├── pty.go                  #   PTY interface
│   │   ├── pty_unix.go             #   openpty(3), TIOCSWINSZ ioctl
│   │   ├── pty_windows.go          #   ConPTY (Windows Pseudo Console API)
│   │   └── process.go              #   spawn shell, waitpid, SIGCHLD
│   │
│   ├── vte/                        # ← copa (Rio) — thin adapter over go-vte library
│   │   ├── performer.go            #   implements vte.Performer (library interface)
│   │   └── sequences.go            #   OSC / DCS dispatch helpers
│   │
│   ├── grapheme/                   # ← rio-grapheme-width (Rio v0.4)
│   │   └── width.go                #   Unicode grapheme cluster width (EAW, ZWJ)
│   │
│   ├── terminal/                   # ← rio-backend
│   │   ├── terminal.go             #   Terminal: owns Grid + cursor + modes
│   │   ├── grid.go                 #   Grid[rows][cols]Cell, resize, scroll
│   │   ├── cell.go                 #   Cell{Char, Fg, Bg, Attrs, Width, Hyperlink}
│   │   ├── color.go                #   Color (Named16 / Indexed256 / TrueColor)
│   │   ├── screen.go               #   primary ↔ alt screen switch
│   │   ├── scrollback.go           #   ring buffer (configurable, default 10 000 lines)
│   │   ├── selection.go            #   Cell / Line / Block / Semantic selection
│   │   ├── hyperlink.go            #   OSC 8 hyperlink table
│   │   └── modes.go                #   terminal mode flags (DEC private modes etc.)
│   │
│   ├── renderer/                   # ← sugarloaf
│   │   ├── renderer.go             #   Renderer — owns GPU resources, drives passes
│   │   ├── sugar.go                #   Sugar, SugarRun types
│   │   ├── batch.go                #   geometry builder + line-hash dedup
│   │   ├── pipeline.go             #   wgpu RenderPipeline setup
│   │   ├── atlas/
│   │   │   ├── atlas.go            #   GlyphAtlas (4096×4096 R8 texture)
│   │   │   └── packer.go           #   Shelf bin-packing algorithm
│   │   ├── font/
│   │   │   ├── loader.go           #   fontconfig / CoreText / DirectWrite
│   │   │   ├── rasterizer.go       #   freetype2 or sfnt rasterizer
│   │   │   └── metrics.go          #   cell width/height, baseline
│   │   └── shaders/
│   │       ├── cell.wgsl           #   Pass 1: background solid rects
│   │       └── glyph.wgsl          #   Pass 2: atlas-sampled glyph quads
│   │
│   ├── config/                     # TOML configuration
│   │   ├── config.go               #   Config struct (mirrors Rio's TOML schema)
│   │   └── defaults.go
│   │
│   ├── watcher/                    # ← rio-notifier
│   │   └── watcher.go              #   config file hot-reload (fsnotify)
│   │
│   └── app/                        # ← frontends/rioterm
│       ├── app.go                  #   App: gogpu.App wrapper, goroutine wiring
│       ├── tabs.go                 #   Tab management (Title, Terminal, PTY)
│       ├── split.go                #   Pane splits (Horizontal / Vertical)
│       └── keybindings.go          #   action dispatch
│
├── go.mod
└── go.sum
```

**Invariants mirrored from Rio:**
- `internal/vte` has zero knowledge of `internal/renderer`. It only calls `Performer` methods.
- `internal/terminal` has zero knowledge of `internal/renderer`. It exposes `Grid()`.
- `internal/renderer` consumes `[][]Cell` — no PTY, no VTE state.
- `internal/app` is the only package that wires all layers together.

---

## 6. Decision 3 — Sugar Renderer

### The Sugar Concept (from sugarloaf)

Rio's renderer operates on two types:

```go
// Sugar — one terminal cell's render data
type Sugar struct {
    Char  rune
    Fg    [4]float32   // RGBA, pre-normalised
    Bg    [4]float32
    Attrs CellAttrs    // bitfield: Bold|Italic|Underline|Strike|Dim|Blink|Reverse
    Width uint8        // 1 = normal, 2 = wide CJK
}

// SugarRun — a horizontal span of cells sharing the same background
// maps to one background rect (Pass 1) + N glyph quads (Pass 2)
type SugarRun struct {
    Sugars []Sugar
    X, Y   float32     // top-left in pixels
}
```

### Line Hash De-duplication

The key optimisation — identical to Rio's approach:

```go
type Renderer struct {
    lineHashes []uint64      // xxhash of each rendered line
    batch      RenderBatch
    // ...
}

func (r *Renderer) BuildFrame(grid *terminal.Grid) {
    r.batch.Reset()

    for row := 0; row < grid.Rows(); row++ {
        line := grid.Line(row)
        h := hashLine(line)                  // xxhash.Sum64 of cell bytes

        if h == r.lineHashes[row] {
            continue                         // ← nothing changed, skip entirely
        }
        r.lineHashes[row] = h
        r.buildLineGeometry(row, line)       // emit CellVertex + GlyphVertex
    }

    r.uploadAndDraw()                        // one GPU submit per frame
}

func hashLine(line []terminal.Cell) uint64 {
    // cast []Cell to []byte via unsafe, hash in one call — no alloc
    return xxhash.Sum64(cellsAsBytes(line))
}
```

### Two-Pass GPU Pipeline (mirrors sugarloaf v0.2 architecture)

```
Frame N:
┌────────────────────────────────────────────────────────────────┐
│  CPU: BuildFrame()                                             │
│   for each dirty line → emit CellVertex[] + GlyphVertex[]     │
│   upload vertex buffers to GPU                                 │
└────────────────────────────────────────────────────────────────┘
                          ↓ one RenderPass
┌────────────────────────────────────────────────────────────────┐
│  GPU Pass 1: cell pipeline                                     │
│   draw N background rectangles (one per run)                  │
│   WGSL: vertex colour → fragment output                        │
├────────────────────────────────────────────────────────────────┤
│  GPU Pass 2: glyph pipeline                                    │
│   draw M glyph quads, sample atlas.r channel for alpha        │
│   WGSL: atlas.r * fg_color → blended output                   │
└────────────────────────────────────────────────────────────────┘
```

### WGSL Shaders

**cell.wgsl** — Pass 1, background rectangles:

```wgsl
struct Vert { @location(0) pos: vec2<f32>, @location(1) color: vec4<f32> }
struct Frag { @builtin(position) pos: vec4<f32>, @location(0) color: vec4<f32> }

@vertex fn vs(v: Vert) -> Frag {
    return Frag(vec4(v.pos, 0.0, 1.0), v.color);
}
@fragment fn fs(f: Frag) -> @location(0) vec4<f32> { return f.color; }
```

**glyph.wgsl** — Pass 2, atlas-sampled glyphs:

```wgsl
@group(0) @binding(0) var atlas:   texture_2d<f32>;
@group(0) @binding(1) var samp:    sampler;
@group(0) @binding(2) var<uniform> transform: mat4x4<f32>;

struct Vert { @location(0) pos: vec2<f32>, @location(1) uv: vec2<f32>,
              @location(2) color: vec4<f32> }
struct Frag { @builtin(position) pos: vec4<f32>,
              @location(0) uv: vec2<f32>, @location(1) color: vec4<f32> }

@vertex fn vs(v: Vert) -> Frag {
    return Frag(transform * vec4(v.pos, 0.0, 1.0), v.uv, v.color);
}
@fragment fn fs(f: Frag) -> @location(0) vec4<f32> {
    let alpha = textureSample(atlas, samp, f.uv).r;
    return vec4(f.color.rgb, f.color.a * alpha);
}
```

### Glyph Atlas

```go
type GlyphAtlas struct {
    texture  gogpu.Texture     // R8Unorm, 4096×4096
    packer   *ShelfPacker      // simple shelf bin-packing, O(1) insert
    cache    map[GlyphKey]GlyphInfo
    dirty    bool              // needs upload to GPU
}

type GlyphKey struct {
    Rune   rune
    Bold   bool
    Italic bool
    Size   float32            // in points, snapped to 0.5pt
    FontID uint8
}

type GlyphInfo struct {
    UV       [4]float32       // {u0,v0,u1,v1} normalised
    BearingX float32
    BearingY float32
    Advance  float32
}
```

When the atlas is full: double its height (up to 8192), then evict LRU glyphs.
This matches sugarloaf's approach of gracefully expanding before evicting.

---

## 7. Decision 4 — PTY & Event Loop

### PTY Layer

```go
// internal/pty/pty.go
type PTY interface {
    Read(p []byte) (int, error)
    Write(p []byte) (int, error)
    Resize(cols, rows, pixW, pixH uint16) error
    Fd() uintptr
    Close() error
}

// Unix: openpty(3) → forkpty → exec shell
// Windows: CreatePseudoConsole (ConPTY) → CreateProcess
```

Key implementation details that match Rio's teletypewriter:
- Non-blocking reads via `epoll` (Linux) / `kqueue` (macOS/BSD) / `IOCP` (Windows)
- `TIOCSWINSZ` ioctl on resize, followed by `SIGWINCH` to child process
- `SIGCHLD` handler to detect shell exit and close the tab cleanly

### Event Loop (replaces corcovado)

Rio uses `corcovado` (a fork of mio) for async I/O. In Go, goroutines and channels
are the natural equivalent:

```go
// internal/app/app.go

func (a *App) Run() error {
    ptyCh    := make(chan []byte, 256)   // raw bytes from PTY
    resizeCh := make(chan Resize,  4)    // window resize events

    // Goroutine 1: PTY reader (blocks on read, fast path)
    go func() {
        buf := make([]byte, 32*1024)
        for {
            n, err := a.pty.Read(buf)
            if err != nil { return }
            chunk := make([]byte, n)
            copy(chunk, buf[:n])
            ptyCh <- chunk
        }
    }()

    // Goroutine 2: VTE parser + grid update (off render thread)
    go func() {
        for data := range ptyCh {
            a.vte.Parse(data)           // → calls Handler → updates Grid
            a.gogpuApp.RequestRedraw()  // signal render thread
        }
    }()

    // gogpu render thread (main): handles Draw + input events
    a.gogpuApp.OnDraw(func(dc *gogpu.Context) {
        a.renderer.Frame(dc, a.terminal.Grid())
    })

    a.gogpuApp.OnKeyPress(func(key input.Key, mods input.Modifiers) {
        seq := a.keybindings.Translate(key, mods)
        a.pty.Write(seq)
    })

    a.gogpuApp.OnResize(func(w, h int) {
        cols, rows := a.renderer.CellsForSize(w, h)
        a.pty.Resize(uint16(cols), uint16(rows), uint16(w), uint16(h))
        a.terminal.Resize(cols, rows)
        a.renderer.Resize(w, h)
    })

    return a.gogpuApp.Run()
}
```

**Thread safety**: `Grid` is protected by a `sync.RWMutex`. The VTE goroutine holds the
write lock during `Parse`; the render goroutine holds the read lock during `Frame`.
Lock contention is minimal because `Parse` batches updates and `Frame` is called at vsync.

---

## 8. Decision 5 — VTE Parser

### Approach — own parser modelled on go-vte  ✅  (mirrors Rio's copa strategy)

Rio does not use `alacritty/vte` as-is — it forks it into `copa` and extends it.
We follow the same pattern: **`internal/vte` is our own Go parser**, built by porting
`go-vte` (a Go translation of `alacritty/vte`).

This gives us:
- A complete, fuzz-tested [Paul Williams VT500 state machine](https://vt100.net/emu/dec_ansi_parser) as the starting point.
- Full control to extend the automaton (e.g. Kitty keyboard protocol, Sixel) without
  waiting on upstream.
- No runtime library dependency — the parser is part of the module.

### Package layout

```
internal/vte/
  parser.go       — Paul Williams automaton (ported from go-vte / alacritty/vte)
  performer.go    — Performer interface + terminal adapter
  sequences.go    — OSC / DCS dispatch helpers
  params.go       — parameter parsing (;: separated uint16 sub-params)
```

### Performer Interface

```go
// internal/vte/performer.go

// Performer receives callbacks from the parser — identical contract to go-vte.
type Performer interface {
    Print(r rune)
    Execute(b byte)
    CSI(params [][]uint16, intermediates []byte, ignore bool, final rune)
    OSC(params [][]byte, bellTerminated bool)
    DCS(params [][]uint16, intermediates []byte, ignore bool, final rune)
    DCSPut(b byte)
    DCSUnhook()
}

// TermPerformer bridges parser callbacks into internal/terminal state updates.
type TermPerformer struct {
    term *terminal.Terminal
}

var _ Performer = (*TermPerformer)(nil)

func (p *TermPerformer) Print(r rune)    { p.term.Print(r) }
func (p *TermPerformer) Execute(b byte)  { p.term.Execute(b) }

func (p *TermPerformer) CSI(params [][]uint16, intermediates []byte, ignore bool, final rune) {
    p.term.CSI(params, intermediates, ignore, final)
}

func (p *TermPerformer) OSC(params [][]byte, bellTerminated bool) {
    p.term.OSC(params, bellTerminated)
}

func (p *TermPerformer) DCS(params [][]uint16, intermediates []byte, ignore bool, final rune) {}
func (p *TermPerformer) DCSPut(b byte) {}
func (p *TermPerformer) DCSUnhook()    {}
```

Usage in the PTY read goroutine:

```go
parser    := vte.New()
performer := &vte.TermPerformer{Term: terminal}

for data := range ptyCh {
    parser.Advance(performer, data)   // zero-alloc hot path
    app.RequestRedraw()
}
```

### Strategy comparison

| Criterion         | Pure hand-roll            | go-vte library only     | **Own parser (Rio strategy)** |
|-------------------|---------------------------|-------------------------|-------------------------------|
| Correctness base  | Error-prone from zero     | Fuzz-tested upstream    | ✅ Fuzz-tested starting point  |
| Flexibility       | Full                      | Limited to library API  | ✅ Full — we own the code      |
| Extensions        | Easy                      | Fork or patch upstream  | ✅ Easy — extend in-tree       |
| Maintenance       | We own all edge cases     | Upstream does it        | Shared: port fixes as needed  |
| **Verdict**       | Too risky                 | Loses flexibility       | ✅ Chosen                      |

### Sequence Priority (implemented in `internal/terminal`, not the parser)

```
PHASE 1 — MVP (terminal is usable):
  SGR (m)              colours: Named16, Indexed256, TrueColor
                       attrs: Bold, Dim, Italic, Underline, Blink, Reverse, Strike
  CUU/CUD/CUF/CUB      cursor movement (A/B/C/D)
  CUP / HVP (H/f)      cursor position
  ED  (J 0/1/2/3)      erase display
  EL  (K 0/1/2)        erase line
  SU / SD (S/T)        scroll up/down
  IL / DL (L/M)        insert/delete lines
  ICH / DCH (@ /P)     insert/delete characters
  RI (ESC M)           reverse index
  SM/RM ?1049          alternate screen
  SM/RM ?25            cursor visibility
  SM/RM ?7             auto-wrap
  DECSTBM (r)          scroll region
  OSC 0/1/2            window title
  OSC 52               clipboard (read/write)

PHASE 2 — Mouse + Links:
  SM/RM ?1000          mouse reporting (button events)
  SM/RM ?1002          mouse reporting (motion while pressed)
  SM/RM ?1006          SGR mouse encoding
  OSC 8               hyperlinks (RFC)

PHASE 3 — Rich content:
  DCS (Sixel)          raster images in terminal
  Kitty image protocol inline images
  Kitty keyboard protocol (extended key reporting)
  DECRQM / XTVERSION  terminal identity

PHASE 4 — Advanced:
  REP (b)              repeat preceding character
  XTPUSHCOLORS / XTPOPCOLORS
  ConEmu / iTerm2 protocol extensions
```

---

## 9. Target Metrics

Derived from Rio's published numbers, community vtebench data, and input latency
measurements. Rio is the ceiling; Alacritty is the comparison point.

| Metric                            | Alacritty  | Rio (Rust)  | **Go Target**   |
|-----------------------------------|------------|-------------|-----------------|
| Input latency                     | ~58 ms     | ~45 ms est. | **< 50 ms**     |
| Frame time, idle 80×24            | ~0.3 ms    | ~0.5 ms     | **< 3 ms**      |
| Frame time, full dirty 80×24      | ~1 ms      | ~2 ms       | **< 8 ms**      |
| Frame time, 136 cols full dirty   | —          | ~8 ms       | **< 20 ms**     |
| `sugar.stack()` equivalent        | —          | ~51.5 µs    | **< 200 µs**    |
| PTY throughput (vtebench)         | very high  | good        | **> 150 MB/s**  |
| Startup time                      | ~200 ms    | ~50 ms      | **< 200 ms**    |
| RSS memory (idle)                 | ~25 MB     | ~30 MB      | **< 80 MB**     |
| Max scrollback lines (configurable)| 10 000    | 10 000      | **10 000**      |

The Go target is conservative: ~3–4× slower than Rio's Rust numbers, which reflects
realistic overhead from the GC, interface dispatch, and goroutine scheduling. We
expect to tighten this as we profile.

---

## 10. Dependencies

### Core (required, Zero CGO)

```
github.com/gogpu/gogpu              GPU framework: Metal/Vulkan/DX12, window, input
github.com/gogpu/wgpu               Pure Go WebGPU (transitive via gogpu)
golang.org/x/sys                    PTY ioctl, syscall, Windows ConPTY
github.com/BurntSushi/toml          config parsing
github.com/zeebo/xxhash             line hashing (zero-alloc)
golang.org/x/image/font/sfnt        pure-Go font loading (SFNT/OTF/TTF)
```

### Optional Performance Upgrades

```
github.com/golang/freetype          better subpixel glyph rasterisation (CGO)
github.com/fsnotify/fsnotify        config hot-reload  ← rio-notifier equivalent
```

### Build Tags

```bash
# Development: pure Go, zero external deps, works in CI/Docker/WASM
CGO_ENABLED=0 go build ./cmd/goterm

# Production release: wgpu-native Rust backend (same binary Rio uses)
CGO_ENABLED=0 go build -tags rust ./cmd/goterm
# Requires wgpu_native.{so,dylib,dll} next to the binary

# Headless / testing: Software renderer, no GPU required
GOGPU_GRAPHICS_API=software go test ./...
```

---

## 11. Implementation Phases

```
Phase 1  Weeks 1–3    PTY + VTE adapter
         ─────────────────────────────────────────────────────────
         Deliverables: internal/pty, internal/vte (Performer), internal/grapheme
         Test gate:    echo / ls --color / htop render correct ANSI
                       vttest Phase 1 (cursor movement) passes
                       ConPTY smoke test on Windows

Phase 2  Weeks 4–6    Terminal Model
         ─────────────────────────────────────────────────────────
         Deliverables: internal/terminal (grid, screen, scrollback, selection)
         Test gate:    vim opens and edits a file (no renderer yet)
                       alternate screen switch works (vim ↔ shell)
                       resize does not corrupt grid content

Phase 3  Weeks 7–9    Software Renderer (CPU, debug only)
         ─────────────────────────────────────────────────────────
         Deliverables: internal/renderer (software path: writes PPM/PNG per frame)
         Test gate:    screenshot of rendered 80×24 grid matches expected PNG

Phase 4  Weeks 10–15  GPU Renderer
         ─────────────────────────────────────────────────────────
         Deliverables: internal/renderer full (atlas, two-pass pipeline, WGSL shaders)
         Test gate:    triangle on screen → one glyph → full text grid
                       line hash dedup: frametime drops ≥ 50% on idle screen
                       glyph atlas: no visible corruption at HiDPI (2×)

Phase 5  Weeks 16–18  Window + Interactive Loop
         ─────────────────────────────────────────────────────────
         Deliverables: internal/app, gogpu event wiring, keybindings
         Test gate:    type in shell → output appears on screen
                       resize window → terminal reflowed correctly
                       CPU usage < 1% when idle

Phase 6  Weeks 19–21  Config, Tabs, Splits
         ─────────────────────────────────────────────────────────
         Deliverables: internal/config, internal/watcher, tabs, splits in internal/app
         Test gate:    TOML config reloads without restart
                       multiple tabs with independent PTYs

Phase 7  Weeks 22–24  Sixel, Hyperlinks, VI Mode, Search
Phase 8  Weeks 25+    Profiling, optimisation to target metrics
```

---

## 12. Rejected Alternatives

### Alacritty as codebase base (fork)
Rejected. Alacritty is Rust; we want Go. Forking and transpiling is more work than a
clean Go implementation guided by Rio's architecture. Alacritty also intentionally lacks
tabs, splits, and images — features we want from the start.

### Electron / web-based terminal (xterm.js)
Rejected. 150 MB+ binary, 200+ ms latency, not GPU-native in the wgpu sense.
Completely different performance profile.

### `fyne.io` as the window/GPU layer
Rejected. Fyne uses OpenGL internally and does not expose a raw WebGPU `Device`.
Impossible to implement the Sugar pipeline without direct control over render passes
and vertex buffers.

### `cogentcore/webgpu` (CGO → wgpu-native)
Rejected. CGO is a hard requirement. Breaks `CGO_ENABLED=0`, complicates Docker and
CI, prevents fully static binaries, adds platform-specific `.so`/`.dylib` dependencies
at runtime.

### Go + pure OpenGL (`go-gl`)
Rejected. OpenGL is deprecated on macOS since Mojave (2018). No compute shaders.
No WebGPU compatibility for WASM target. Closes the Retro shader path. We would be
building on a shrinking API surface.

### `gogpu/wgpu` directly (without `gogpu/gogpu`)
Rejected. `gogpu/wgpu` is a pure GPU compute library — no window manager, no input
handling, no surface management. We would have to implement Win32 / Cocoa / X11 /
Wayland windowing ourselves. This is 3–6 months of work already done in `gogpu/gogpu`.

---

## 13. Risks & Mitigations

| Risk                                          | Likelihood | Mitigation                                              |
|-----------------------------------------------|------------|---------------------------------------------------------|
| `gogpu` bug on Apple Silicon Metal path       | Medium     | `-tags rust` → wgpu-native fallback; test on M-series in CI |
| Go GC pause causes frame stutter              | Medium     | `runtime.LockOSThread()` on render goroutine; `sync.Pool` for vertex slices; tune `GOGC` |
| go-vte library missing edge cases / unmaintained | Low–Med  | vttest suite in CI from Phase 1; Performer adapter isolates parser — swappable |
| Pure Go wgpu throughput insufficient          | High       | Accepted trade-off: dev = Pure Go, release = `-tags rust` |
| Font rasterisation quality (sfnt vs freetype) | Medium     | A/B test both; sfnt for zero-CGO default, freetype opt-in |
| ConPTY (Windows) edge cases                   | Medium     | Windows CI runner; mirror Rio's teletypewriter Windows code |
| Goroutine-to-render-thread synchronisation    | Low        | `sync.RWMutex` on Grid; profile contention with `pprof` |

---

## 14. References

| Resource                         | URL                                                              |
|----------------------------------|------------------------------------------------------------------|
| Rio Terminal (primary reference) | https://github.com/raphamorim/rio                               |
| Rio Cargo.toml (v0.4.6)          | https://github.com/raphamorim/rio/blob/main/Cargo.toml          |
| Rio release notes (perf numbers) | https://github.com/raphamorim/rio/blob/main/docs/docs/releases.md |
| Rio v0.0.15 Sugar dedup numbers  | https://rioterm.com/blog/2023/08/02/release-0.0.15              |
| Sugarloaf renderer (archived)    | https://github.com/raphamorim/sugarloaf                         |
| gogpu/gogpu                      | https://github.com/gogpu/gogpu                                  |
| gogpu/wgpu                       | https://github.com/gogpu/wgpu                                   |
| Paul Williams VT500 state machine| https://vt100.net/emu/dec_ansi_parser                           |
| go-vte (porting reference)       | https://github.com/nicowillis/go-vte                            |
| alacritty/vte (original Rust)    | https://github.com/alacritty/vte                                |
| vtebench (Alacritty's tool)      | https://github.com/alacritty/vtebench                           |
| Input latency measurements       | https://dev.to/lkhrs/measuring-terminal-latency-26m7            |
| Ghostty performance discussion   | https://github.com/ghostty-org/ghostty/discussions/4837         |
