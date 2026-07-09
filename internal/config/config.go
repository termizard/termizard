// Package config loads and holds user-configurable terminal settings.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	TitleAlignLeft   = "left"
	TitleAlignCenter = "center"
	TitleAlignRight  = "right"

	TabLabelPath  = "path"
	TabLabelIndex = "index"

	TabSizeFull    = "full"    // stretch tabs across the bar (flex:1)
	TabSizeCompact = "compact" // fixed-width slots (~140px), like JetBrains
)

// Config holds all user-configurable terminal settings.
type Config struct {
	Window      WindowConfig
	Tabs        TabsConfig
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
	TitlePosition string `toml:"title_position"`
	Width         int
	Height        int
	MinWidth      int
	MinHeight     int
	Opacity       float64
	PaddingX      int  `toml:"padding_x"`
	PaddingY      int  `toml:"padding_y"`
	ShowTitleBar  bool `toml:"show_title_bar"`
}

type TabsConfig struct {
	Enabled        bool
	Label          string `toml:"label"` // path | index
	Size           string `toml:"size"`  // full | compact
	ShowNewButton  bool   `toml:"show_new_button"`
	ShowWhenSingle bool   `toml:"show_when_single"`
}

// TabItem holds optional per-tab shell overrides (reserved for future use).
type TabItem struct {
	Title   string
	Cwd     string
	Program string
	Args    []string
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
	Program   string   `toml:"program"`
	Args      []string `toml:"args"`
	NoOhMyZsh bool     `toml:"no_oh_my_zsh"` // skip user rc; use bundled prompt when prompt = "kali"
	Prompt    string   `toml:"prompt"`       // kali | none — twoline box prompt when no_oh_my_zsh
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

const minimalDefaultConfig = `# termizard user config
# Full reference: https://github.com/termizard/termizard/blob/main/config.example.toml

[window]
show_title_bar = true
`

// EnsureDefaultFile writes a minimal config template when the file does not exist yet.
func EnsureDefaultFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec
		return err
	}
	return os.WriteFile(path, []byte(minimalDefaultConfig), 0o644) //nolint:gosec
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
	normalize(cfg)
	return cfg, nil
}

func normalize(cfg *Config) {
	cfg.Window.TitlePosition = normalizeTitlePosition(cfg.Window.TitlePosition)
	cfg.Tabs.Label = normalizeTabLabel(cfg.Tabs.Label)
	cfg.Tabs.Size = normalizeTabSize(cfg.Tabs.Size)
}

func normalizeTitlePosition(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case TitleAlignLeft, TitleAlignCenter, TitleAlignRight:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return TitleAlignCenter
	}
}

func normalizeTabLabel(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case TabLabelPath, TabLabelIndex:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return TabLabelPath
	}
}

func normalizeTabSize(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case TabSizeFull, TabSizeCompact:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return TabSizeCompact
	}
}

// TabsActive reports whether in-window tabbing is enabled.
func (cfg *Config) TabsActive() bool {
	return cfg.Tabs.Enabled
}

// DefaultPath returns the XDG-compliant config file path.
func DefaultPath() string {
	return filepath.Join(configHome(), "termizard", "config.toml")
}

func configHome() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return base
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config")
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		return filepath.Join(home, ".config")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}
