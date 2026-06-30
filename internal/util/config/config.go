package config

import (
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
	Title     string
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
	Opacity   float64
	PaddingX  int // horizontal inset between window edge and first cell column
	PaddingY  int // vertical inset between window edge and first cell row
}

type TerminalConfig struct {
	InitialCols int
	InitialRows int
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
	Program string
	Args    []string
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
	Action string // "Paste" | "Copy" | "ScrollUp" | "ScrollDown"
}

// Load reads the config file at path and merges it over Defaults().
// A missing file is not an error — defaults are returned.
func Load(path string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
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

// Defaults returns a Config with sensible out-of-the-box values.
func Defaults() *Config {
	return &Config{
		Window: WindowConfig{
			Title:     "termizard",
			Width:     1200,
			Height:    800,
			MinWidth:  400,
			MinHeight: 240,
			Opacity:   1.0,
			PaddingX:  6,
			PaddingY:  6,
		},
		Terminal: TerminalConfig{
			InitialCols: 80,
			InitialRows: 24,
		},
		Font: FontConfig{
			Size: 14.0,
		},
		Colors: ColorConfig{
			Background: "#1a1a2e",
			Foreground: "#e0e0e0",
			Cursor:     "#f38ba8",
			Selection:  "#44475a",
			ANSI: ANSIColors{
				Black:         "#000000",
				Red:           "#ff5555",
				Green:         "#50fa7b",
				Yellow:        "#f1fa8c",
				Blue:          "#bd93f9",
				Magenta:       "#ff79c6",
				Cyan:          "#8be9fd",
				White:         "#bfbfbf",
				BrightBlack:   "#4d4d4d",
				BrightRed:     "#ff6e67",
				BrightGreen:   "#5af78e",
				BrightYellow:  "#f4f99d",
				BrightBlue:    "#caa9fa",
				BrightMagenta: "#ff92d0",
				BrightCyan:    "#9aedfe",
				BrightWhite:   "#e6e6e6",
			},
		},
		Shell:      ShellConfig{},
		Scrollback: ScrollbackConfig{Lines: 10000},
		Cursor:     CursorConfig{Shape: "block"},
		Keybindings: []Keybinding{
			{Key: "v", Mods: []string{"Ctrl", "Shift"}, Action: "Paste"},
			{Key: "c", Mods: []string{"Ctrl", "Shift"}, Action: "Copy"},
			{Key: "Up", Mods: []string{"Ctrl", "Shift"}, Action: "ScrollUp"},
			{Key: "Down", Mods: []string{"Ctrl", "Shift"}, Action: "ScrollDown"},
		},
	}
}
