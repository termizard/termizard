package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/shell/prompt"
)

func TestMaterializeWritesPromptFiles(t *testing.T) {
	zshDir, bashRC, err := prompt.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(zshDir, ".zshrc")); err != nil {
		t.Fatalf(".zshrc missing in %s: %v", zshDir, err)
	}
	if _, err := os.Stat(filepath.Join(zshDir, "kali-prompt.zsh")); err != nil {
		t.Fatalf("kali-prompt.zsh missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(zshDir, "kali-prompt.zsh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "┌──") || !strings.Contains(string(data), "└─") {
		t.Fatalf("kali-prompt.zsh missing layout markers: %q", data)
	}
	if _, err := os.Stat(bashRC); err != nil {
		t.Fatalf("bash rc missing: %v", err)
	}
}

func TestApplyEnvZshSetsZdotdir(t *testing.T) {
	env, err := prompt.ApplyEnv([]string{"HOME=/tmp"}, "/bin/zsh", prompt.StyleKali)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "ZDOTDIR=") && strings.TrimPrefix(kv, "ZDOTDIR=") != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ZDOTDIR, got %v", env)
	}
}

func TestApplyEnvNoneIsNoop(t *testing.T) {
	base := []string{"HOME=/tmp", "TERM=xterm-256color"}
	env, err := prompt.ApplyEnv(base, "/bin/zsh", prompt.StyleNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != len(base) {
		t.Fatalf("env changed: %v", env)
	}
}

func TestApplyEnvPowerShellSetsProfile(t *testing.T) {
	env, err := prompt.ApplyEnv([]string{"HOME=/tmp"}, "pwsh.exe", prompt.StyleKali)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERMIZARD_PS_PROFILE=") && strings.TrimPrefix(kv, "TERMIZARD_PS_PROFILE=") != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TERMIZARD_PS_PROFILE, got %v", env)
	}
}
