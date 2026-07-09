//go:build manual

package pty

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestManualZshPromptOnResize(t *testing.T) {
	p, err := Open(Config{
		Command: []string{"/bin/zsh", "-i"},
		Cols:    120,
		Rows:    40,
		Env:     os.Environ(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	buf := make([]byte, 4096)
	read := func(label string) string {
		time.Sleep(300 * time.Millisecond)
		var out []byte
		for i := 0; i < 10; i++ {
			n, _ := p.Read(buf)
			out = append(out, buf[:n]...)
			time.Sleep(50 * time.Millisecond)
		}
		s := string(out)
		t.Logf("=== %s ===\n%q", label, s)
		return s
	}

	s1 := read("after open")
	_ = p.Resize(120, 35)
	s2 := read("after resize")

	countPrompts := func(s string) int {
		lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
		n := 0
		for _, l := range lines {
			if strings.Contains(l, "➜") || strings.Contains(l, "❯") || strings.HasSuffix(strings.TrimSpace(l), "%") {
				n++
				t.Logf("  prompt line: %q", l)
			}
		}
		return n
	}
	t.Logf("prompts after open: %d", countPrompts(s1))
	t.Logf("prompts after resize: %d", countPrompts(s2))
}
