<p align="center">
  <img src="assets/logo.png" alt="termizard logo" width="180" />
</p>

<h1 align="center">termizard</h1>

<p align="center">Terminal emulator written in Go — Wails v3 + xterm.js UI, PTY-backed shell.</p>

## Requirements

- Go 1.26+
- For local dev builds: Node.js 22+ (or Docker — see below)
- For release installers: [Task](https://taskfile.dev), [wails3](https://v3.wails.io) (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`)

## Quick start

```bash
# Build frontend (Docker, no local Node required) + Go binary
make build
./termizard
```

Config is optional — defaults are used if missing:

`~/.config/termizard/config.toml`

```toml
[window]
show_title_bar = true

[shell]
# no_oh_my_zsh = true   # bare zsh for testing
```

## Development

```bash
# Tests
go test ./...

# Lint
golangci-lint run --timeout=5m

# Frontend only (Docker)
make frontend

# Go binary only (when dist/ is already built)
make build-go

# Wails dev mode (hot reload)
task dev
```

## Release packaging

Installers use `assets/logo.png` for app icons and desktop shortcuts.

```bash
# macOS — binary + DMG (drag to Applications)
task darwin:package ARCH=arm64

# Windows — NSIS installer (desktop + Start Menu shortcuts)
task windows:package ARCH=amd64 INSTALL_SCOPE=user

# Linux — .deb + AppImage (menu + desktop shortcuts)
task linux:create:deb ARCH=amd64
task linux:create:appimage ARCH=amd64
```

Artifacts land in `bin/`. Publishing to GitHub Releases is handled by [`.github/workflows/release.yml`](.github/workflows/release.yml) on release publish.

## Project layout

```
cmd/termizard/                 Application entry point
frontend/                      React + xterm.js UI (Vite); embeds dist/ at build time
internal/
  adapter/                     UI / PTY adapter interfaces
  app/                         Wires PTY, config, and UI
  config/                      User config (TOML)
  core/
    pty/                       Pseudo-terminal
    terminal/                  Terminal grid & screen model
    vte/                       VT escape-sequence parser
  ui/
    wails/                     Wails v3 backend service
    mock/                      Test doubles
assets/                        Static assets (logo.png)
build/                         Packaging: Taskfiles, NSIS, nfpm, DMG script
.github/workflows/             CI and release automation
```

Generated locally and not committed: `bin/`, `.task/`, `frontend/dist/`, generated icons under `build/`.

## Pre-release check

```bash
bash scripts/pre-release-check.sh
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
