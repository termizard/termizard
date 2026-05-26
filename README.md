<p align="center">
  <img src="assets/logo.png" alt="TERMizard Logo" width="180" />
</p>

<h1 align="center">TERMizard</h1>

# TERMizard

A terminal emulator and TUI framework written in Go.

## Requirements

- Go 1.26+

## Build

```bash
go build ./...
```

## Run

```bash
go run ./cmd/termizard
```

## Test

```bash
go test ./...
```

## Lint

```bash
golangci-lint run --timeout=5m
```

## Pre-release check

```bash
bash scripts/pre-release-check.sh
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
