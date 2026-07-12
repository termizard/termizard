<p align="center">
  <img src="assets/logo.png" alt="termizard logo" width="180" />
</p>

<h1 align="center">termizard</h1>

<p align="center">GPU-native terminal emulator in Go — gogpu UI, PTY/ConPTY-backed shell, tabs &amp; keybindings.</p>

## Requirements

- Go 1.26+
- For release installers: [Task](https://taskfile.dev)

## Quick start

```bash
go run ./cmd/termizard/
# or
go build -o termizard ./cmd/termizard/ && ./termizard
```

Verbose logs (useful on Windows when debugging ConPTY):

```bash
go run ./cmd/termizard/ -v
```

Config works out of the box — full defaults live in code. On first run a **minimal** file is created at `~/.config/termizard/config.toml` (on Windows: `%USERPROFILE%\.config\termizard\config.toml`). For all options, copy [`config.example.toml`](config.example.toml) and edit explicitly.

```toml
[window]
show_title_bar = true
```

### Windows notes

- Shell default: PowerShell 7 (`pwsh`) when installed, otherwise Windows PowerShell 5, then `cmd.exe`.
- Interactive sessions use ConPTY (`CreatePseudoConsole`). Git Bash `$SHELL` is ignored — set `[shell] program` explicitly if you need another shell.
- Window/tab titles follow the shell working directory (e.g. `C:\Users\you`), matching the path in `PS C:\Users\you>`. ConPTY process-image titles (`…\powershell.exe`) are ignored.
- Live window resize defers full-window GPU texture recreation until the drag ends (Windows `WM_ENTERSIZEMOVE`, macOS/Wayland live-resize). Prevents multi-GB growth from per-tick texture churn.

```toml
# config.toml — optional Windows shell override
[shell]
program = "pwsh.exe"   # or "powershell.exe" / "cmd.exe"
args = []
```

## Development

```bash
# Tests
go test ./...

# Lint
golangci-lint run --timeout=5m

# Cross-compile Windows binary (from macOS/Linux)
GOOS=windows GOARCH=amd64 go build -o termizard.exe ./cmd/termizard/
```

## Release packaging

Installers use `assets/logo.png` for app icons and desktop shortcuts.

```bash
# macOS — binary + DMG (drag to Applications)
task darwin:package ARCH=arm64

# Windows — universal NSIS installer (amd64 + arm64; desktop + Start Menu shortcuts)
task windows:package:universal INSTALL_SCOPE=user
# Artifact: bin/termizard-amd64_arm64-installer.exe

# Linux — .deb + AppImage (menu + desktop shortcuts)
task linux:create:deb ARCH=amd64
task linux:create:appimage ARCH=amd64
```

Artifacts land in `bin/`. Publishing to GitHub Releases is handled by [`.github/workflows/release.yml`](.github/workflows/release.yml) on release publish.

## Project layout

```
cmd/termizard/                 Application entry point
internal/
  adapter/                     UI / PTY adapter interfaces
  app/                         Wires PTY, config, and UI
  config/                      User config (TOML)
  core/
    pty/                       Pseudo-terminal (POSIX + Windows ConPTY)
    terminal/                  Terminal grid & screen model
    vte/                       VT escape-sequence parser
  shell/                       Bundled prompt helpers (optional)
  ui/
    gogpu/                     Native GPU UI (gogpu)
    mock/                      Test doubles
assets/                        Static assets (logo.png)
build/                         Packaging: Taskfiles, NSIS, nfpm, DMG script
.github/workflows/             CI and release automation
```

Generated locally and not committed: `bin/`, `.task/`, generated icons under `build/`.

## Pre-release check

```bash
bash scripts/pre-release-check.sh
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
