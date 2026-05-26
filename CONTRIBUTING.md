# Contributing to TERMizard

Thank you for your interest in contributing to TERMizard!

---

## Requirements

- **Go 1.25+** (required for iterators, generics, and modern features)
- **golangci-lint** for code quality checks
- **Rust toolchain** (optional, for native rendering backend testing)

---

## Quick Start

```bash
# Clone the repository
git clone https://github.com/termizard/termizard
cd termizard

# Build
go build ./...

# Run tests
go test ./...

# Run linter
golangci-lint run --timeout=5m
```

---

## Development Workflow

### 1. Fork & Clone

```bash
git clone https://github.com/YOUR_USERNAME/termizard
cd termizard
git remote add upstream https://github.com/termizard/termizard
```

### 2. Create Feature Branch

```bash
git checkout -b feat/your-feature
# or
git checkout -b fix/issue-number-description
```

### 3. Make Changes

- Follow code style guidelines below
- Add tests for new functionality
- Update documentation if needed

### 4. Validate Before Commit

```bash
# Format code
go fmt ./...

# Run pre-release checks
bash scripts/pre-release-check.sh
```

### 5. Create Pull Request

**All contributions must go through Pull Requests:**

```bash
git add .
git commit -m "feat(component): description"
git push origin feat/your-feature
```

Then open a PR on GitHub: `https://github.com/termizard/termizard/compare`

---

## Pull Request Guidelines

### PR Requirements

- [ ] All tests pass (`go test ./...`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] Code is formatted (`go fmt ./...`)
- [ ] Documentation updated (if applicable)
- [ ] CHANGELOG.md updated (for features/fixes)

### PR Title Format

```
feat(tui): add file explorer widget
fix(term): resolve cursor position on resize
docs: update README with usage examples
test(ptp): add integration tests for pseudo-terminal proxy
chore(ci): add linter step to github actions
refactor(parser): simplify escape sequence handling
```

### PR Description Template

```markdown
## Summary
Brief description of changes.

## Changes
- Change 1
- Change 2

## Testing
How was this tested?

## Related Issues
Closes #123
```

---

## Code Style

### Go Conventions

- Use `gofmt` for formatting (tabs, not spaces)
- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use pointer receivers for structs with mutexes

### Naming

| Type | Convention | Example |
|------|------------|---------|
| Exported | PascalCase | `CreatePseudoTerminal`, `RenderWidget` |
| Unexported | camelCase | `parseEscapeSequence`, `handleResize` |
| Acronyms | Uppercase | `GetPTYSize`, `ProcessTTYSignal`, `EscapeCSI` |
| Constants | PascalCase | `MaxBufferSize`, `DefaultShell` |

### Error Handling

```go
// Always check errors
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Or explicitly ignore
_ = file.Close()
```

---

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

[optional body]

[optional footer]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation |
| `test` | Tests |
| `refactor` | Code refactoring |
| `perf` | Performance |
| `ci` | CI/CD changes |
| `chore` | Maintenance |

### Scopes

| Scope | Description |
|-------|-------------|
| `tui` | Terminal UI components |
| `ptp` | Pseudo-terminal proxy |
| `term` | Terminal emulation core |
| `parser` | Escape sequence parser |
| `widgets` | TUI widgets |
| `render` | Screen rendering |
| `config` | Configuration |
| `deps` | Dependencies |

---

## Project Structure

```
termizard/
├── cmd/                    # Application entry point
│   └── termizard/
├── internal/
└── scripts/                # Build/release scripts
```

---

## #TODO Platform Support

---

## Testing

### Run All Tests

```bash
go test ./...
```

### Run Specific Package

```bash
go test -v ./ptp/...
```

### Run with Race Detector

```bash
go test -race ./...
```

### Pre-Release Validation

```bash
bash scripts/pre-release-check.sh
```

---

Here's the English version for your **termizard** project:

---

## Areas Where We Need Help

- **Platform Testing** — Run and verify behaviour on Linux (X11, Wayland), macOS, Windows Terminal, and across various terminal emulators (Alacritty, WezTerm, Kitty).
- **Terminal Emulation Protocols** — Extended escape sequence support (xterm, kitty image protocol), OSC commands, and terminal notifications.
- **Performance** — Profiling and optimizing escape sequence parsing, screen diff updates, and rendering of complex TUI layouts.
- **Documentation** — Widget usage examples, configuration guides, and API descriptions for core packages.
- **Accessibility** — Screen reader support, high-contrast mode, and integration with system font settings.

---

## Questions?

- Open a [GitHub Issue](https://github.com/termizard/termizard/issues)
- Check existing [Discussions](https://github.com/termizard/termizard/discussions)

---

*Thank you for contributing to TERMizard!*