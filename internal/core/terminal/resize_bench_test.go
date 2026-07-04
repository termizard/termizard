package terminal_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/termizard/termizard/internal/core/terminal"
	"github.com/termizard/termizard/internal/core/vte"
)

func feedBytes(term *terminal.Terminal, data string) {
	p := vte.New()
	p.Advance(term, []byte(data))
}

func BenchmarkResizeDrag(b *testing.B) {
	term := terminal.New(120, 40, 10000, true)
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "drwx------@  user staff %6d Jun 30 23:00 long-filename-entry-%02d\r\n", i*100, i)
	}
	data := sb.String()
	feedBytes(term, data)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for w := 120; w >= 40; w -= 5 {
			term.Resize(w, 40)
		}
		for w := 40; w <= 120; w += 5 {
			term.Resize(w, 40)
		}
	}
}

func TestResizeMemoryGrowth(t *testing.T) {
	term := terminal.New(120, 40, 10000, true)
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "drwx------@  user staff %6d Jun 30 23:00 long-filename-entry-%02d\r\n", i*100, i)
	}
	feedBytes(term, sb.String())

	runtime.GC()
	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)

	for round := 0; round < 50; round++ {
		for w := 120; w >= 40; w -= 5 {
			term.Resize(w, 40)
		}
		for w := 40; w <= 120; w += 5 {
			term.Resize(w, 40)
		}
	}
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	growth := int64(m1.HeapAlloc) - int64(m0.HeapAlloc)
	t.Logf("heap growth after 50 resize cycles: %d bytes (%.1f MB)", growth, float64(growth)/1e6)
	if growth > 50*1024*1024 {
		t.Fatalf("excessive heap growth: %.1f MB", float64(growth)/1e6)
	}
}
