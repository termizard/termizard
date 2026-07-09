package prompt

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	StyleKali = "kali"
	StyleNone = "none"
)

//go:embed embed/zsh/.zshrc embed/kali-prompt.zsh embed/bash/bashrc embed/powershell/profile.ps1
var embedded embed.FS

// Materialize writes bundled prompt files to the user cache dir.
func Materialize() (zshDir, bashRC string, err error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("prompt: cache dir: %w", err)
	}
	root := filepath.Join(base, "termizard", "prompt")

	zshDir = filepath.Join(root, "zsh")
	if err := writeEmbedded("embed/zsh/.zshrc", filepath.Join(zshDir, ".zshrc")); err != nil {
		return "", "", err
	}
	if err := writeEmbedded("embed/kali-prompt.zsh", filepath.Join(zshDir, "kali-prompt.zsh")); err != nil {
		return "", "", err
	}

	bashRC = filepath.Join(root, "bash", "bashrc")
	if err := writeEmbedded("embed/bash/bashrc", bashRC); err != nil {
		return "", "", err
	}
	return zshDir, bashRC, nil
}

// PowerShellProfilePath returns the materialized PowerShell prompt script path.
func PowerShellProfilePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("prompt: cache dir: %w", err)
	}
	dst := filepath.Join(base, "termizard", "prompt", "powershell", "profile.ps1")
	if err := writeEmbedded("embed/powershell/profile.ps1", dst); err != nil {
		return "", err
	}
	return dst, nil
}

// ApplyEnv adds environment variables needed for the bundled prompt style.
func ApplyEnv(base []string, shellPath, style string) ([]string, error) {
	if style == "" {
		style = StyleKali
	}
	if style == StyleNone {
		return base, nil
	}
	if style != StyleKali {
		return base, nil
	}

	zshDir, bashRC, err := Materialize()
	if err != nil {
		return base, err
	}

	switch shellBase(shellPath) {
	case "zsh":
		return setEnv(base, "ZDOTDIR", zshDir), nil
	case "bash":
		return setEnv(base, "TERMIZARD_BASH_RC", bashRC), nil
	case "pwsh", "powershell":
		psProfile, err := PowerShellProfilePath()
		if err != nil {
			return base, err
		}
		return setEnv(base, "TERMIZARD_PS_PROFILE", psProfile), nil
	default:
		return base, nil
	}
}

func shellBase(shellPath string) string {
	base := filepath.Base(shellPath)
	if i := strings.LastIndex(base, "-"); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, prefix+value)
			found = true
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, prefix+value)
	}
	return out
}

func writeEmbedded(src, dst string) error {
	data, err := fs.ReadFile(embedded, src)
	if err != nil {
		return fmt.Errorf("prompt: read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec
		return fmt.Errorf("prompt: mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("prompt: write %s: %w", dst, err)
	}
	return nil
}
