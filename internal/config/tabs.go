package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShellCommand returns argv for this tab's PTY, falling back to global [shell].
func (item TabItem) ShellCommand(cfg *Config) []string {
	tabShell := ShellConfig{
		Program:   item.Program,
		Args:      item.Args,
		NoOhMyZsh: cfg.Shell.NoOhMyZsh,
		Prompt:    cfg.Shell.Prompt,
	}
	if item.Program != "" || len(item.Args) > 0 {
		tmp := *cfg
		tmp.Shell = tabShell
		return tmp.ShellCommand()
	}
	return cfg.ShellCommand()
}

// WorkingDir resolves the tab working directory (empty → home, "~" → home).
func (item TabItem) WorkingDir() string {
	cwd := strings.TrimSpace(item.Cwd)
	if cwd == "" || cwd == "~" {
		if home := os.Getenv("HOME"); home != "" {
			return home
		}
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return ""
	}
	if strings.HasPrefix(cwd, "~/") {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, cwd[2:])
		}
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, cwd[2:])
		}
	}
	return cwd
}

// DisplayTitle returns the tab label for the UI.
func (item TabItem) DisplayTitle(index int) string {
	if t := strings.TrimSpace(item.Title); t != "" {
		return t
	}
	return fmt.Sprintf("Tab %d", index+1)
}
