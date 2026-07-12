package config

import "runtime"

const (
	modCtrl     = "Ctrl"
	modShift    = "Shift"
	goosWindows = "windows"
)

// Defaults returns the out-of-the-box configuration: JetBrains IDE dark theme,
// colors, blinking block cursor, and JetBrains Mono as the preferred font.
func Defaults() *Config {
	// JetBrains New UI editor surface; greenish chrome is painted around blocks in gogpu.
	bg := "#1e1f22"
	fontFamily := `"JetBrains Mono", "Menlo", "SF Mono", "Monaco", "Courier New", monospace`
	if runtime.GOOS == goosWindows {
		fontFamily = `"Cascadia Mono", "Consolas", "Courier New", monospace`
	}
	return &Config{
		Window: WindowConfig{
			Title:         "termizard",
			TitlePosition: TitleAlignCenter,
			Width:         1200,
			Height:        800,
			MinWidth:      400,
			MinHeight:     240,
			Opacity:       1.0,
			PaddingX:      12,
			PaddingY:      12,
			ShowTitleBar:  true,
		},
		Tabs: TabsConfig{
			Enabled:        true,
			Label:          TabLabelPath,
			Size:           TabSizeCompact,
			ShowNewButton:  true,
			ShowWhenSingle: true,
		},
		Terminal: TerminalConfig{
			InitialCols:    80,
			InitialRows:    24,
			ReflowOnResize: true,
		},
		Font: FontConfig{
			Family: fontFamily,
			Size:   16,
		},
		Colors: ColorConfig{
			Background: bg,
			Foreground: "#BCBEC4",
			Cursor:     "#BCBEC4",
			Selection:  "rgba(33, 66, 131, 0.85)",
			ANSI: ANSIColors{
				Black:         "#2B2B2B",
				Red:           "#FF6B68",
				Green:         "#A8C023",
				Yellow:        "#FFC66D",
				Blue:          "#5394EC",
				Magenta:       "#9876AA",
				Cyan:          "#299999",
				White:         "#A9B7C6",
				BrightBlack:   "#555555",
				BrightRed:     "#FF8782",
				BrightGreen:   "#B8E986",
				BrightYellow:  "#FFD37F",
				BrightBlue:    "#6BAFFF",
				BrightMagenta: "#C198CB",
				BrightCyan:    "#3DDAD7",
				BrightWhite:   "#FFFFFF",
			},
		},
		Shell:      ShellConfig{},
		Scrollback: ScrollbackConfig{Lines: 10000},
		Cursor: CursorConfig{
			Shape: "block",
			Blink: true,
		},
		Keybindings: []Keybinding{
			{Key: "v", Mods: []string{modCtrl, modShift}, Action: "Paste"},
			{Key: "c", Mods: []string{modCtrl, modShift}, Action: "Copy"},
			{Key: "k", Mods: []string{modCtrl, modShift}, Action: "Clear"},
			{Key: "Up", Mods: []string{modCtrl, modShift}, Action: "ScrollUp"},
			{Key: "Down", Mods: []string{modCtrl, modShift}, Action: "ScrollDown"},
			{Key: "PageUp", Mods: []string{}, Action: "ScrollPageUp"},
			{Key: "PageDown", Mods: []string{}, Action: "ScrollPageDown"},
			{Key: "Home", Mods: []string{modCtrl}, Action: "ScrollTop"},
			{Key: "End", Mods: []string{modCtrl}, Action: "ScrollBottom"},
			{Key: "=", Mods: []string{modCtrl}, Action: "FontIncrease"},
			{Key: "+", Mods: []string{modCtrl, modShift}, Action: "FontIncrease"},
			{Key: "-", Mods: []string{modCtrl}, Action: "FontDecrease"},
			{Key: "0", Mods: []string{modCtrl}, Action: "FontReset"},
		},
	}
}
