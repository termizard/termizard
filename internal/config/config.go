// Package config loads and holds user-configurable terminal settings.
package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all user-configurable terminal settings.
type Config struct {
	Window      WindowConfig
	Terminal    TerminalConfig
	Font        FontConfig
	Colors      ColorConfig
	Shell       ShellConfig
	Scrollback  ScrollbackConfig
	Cursor      CursorConfig
	Keybindings []Keybinding
}

type WindowConfig struct {
	Title         string
	Width         int
	Height        int
	MinWidth      int
	MinHeight     int
	Opacity       float64
	PaddingX      int
	PaddingY      int
	ShowTitleBar  bool `toml:"show_title_bar"`
}

type TerminalConfig struct {
	InitialCols    int
	InitialRows    int
	ReflowOnResize bool
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
	ANSI       ANSIColors
}

// ANSIColors holds the 16-color ANSI palette overrides.
type ANSIColors struct {
	Black         string
	Red           string
	Green         string
	Yellow        string
	Blue          string
	Magenta       string
	Cyan          string
	White         string
	BrightBlack   string
	BrightRed     string
	BrightGreen   string
	BrightYellow  string
	BrightBlue    string
	BrightMagenta string
	BrightCyan    string
	BrightWhite   string
}

type ShellConfig struct {
	Program     string
	Args        []string
	NoOhMyZsh   bool // skip .zshrc (oh-my-zsh) — bare zsh for testing
}

type ScrollbackConfig struct {
	Lines int
}

type CursorConfig struct {
	Shape string // "block" | "beam" | "underline"
	Blink bool
}

// Keybinding maps a key+mods combination to a named action.
type Keybinding struct {
	Key    string
	Mods   []string
	Action string
}

// EnsureDefaultFile writes Defaults() to path when the file does not exist yet.
func EnsureDefaultFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# termizard default configuration\n")
	buf.WriteString("# https://github.com/termizard/termizard\n\n")
	if err := toml.NewEncoder(&buf).Encode(Defaults()); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec
}

// Load reads the config file at path and merges it over Defaults().
// A missing file is not an error — defaults are returned.
func Load(path string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path) //nolint:gosec // config path is chosen by the user or DefaultPath()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// DefaultPath returns the XDG-compliant config file path.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "termizard", "config.toml")
}
