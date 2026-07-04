package pty

import (
	"os"
	"runtime"
	"strings"
)

// DefaultShell returns the login shell for PTY children when config does not
// override it. GUI-launched macOS apps often have no SHELL in their environment.
func DefaultShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}

// PrepareShellEnv returns an environment suitable for an interactive PTY session.
// GUI app bundles inherit a minimal environment (no TERM, short PATH) which breaks
// shells, themes, and keyboard input when launched from Finder/DMG.
func PrepareShellEnv(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	m := envMap(base)

	setIfEmpty(m, "TERM", "xterm-256color")
	setIfEmpty(m, "COLORTERM", "truecolor")
	setIfEmpty(m, "LANG", defaultLANG())
	setIfEmpty(m, "SHELL", DefaultShell())

	mergePATH(m)

	return envSlice(m)
}

func defaultLANG() string {
	for _, key := range []string{"LANG", "LC_CTYPE", "LC_ALL"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return "en_US.UTF-8"
}

func mergePATH(m map[string]string) {
	existing := m["PATH"]
	parts := splitPath(existing)

	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		seen[p] = struct{}{}
	}

	for _, p := range standardPathEntries() {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		if info, err := os.Stat(p); err != nil || !info.IsDir() {
			continue
		}
		parts = append(parts, p)
		seen[p] = struct{}{}
	}

	m["PATH"] = strings.Join(parts, string(os.PathListSeparator))
}

func standardPathEntries() []string {
	home, _ := os.UserHomeDir()
	entries := []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	}
	if home != "" {
		entries = append([]string{
			home + "/.local/bin",
			home + "/bin",
		}, entries...)
	}
	return entries
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, string(os.PathListSeparator))
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env)+4)
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		m[k] = v
	}
	return m
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func setIfEmpty(m map[string]string, key, value string) {
	if value == "" {
		return
	}
	if cur, ok := m[key]; !ok || cur == "" {
		m[key] = value
	}
}
