package gogpu

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
)

// fontBundle holds the primary cell face plus an optional symbol fallback
// (JetBrains / SF Mono have → ❯ which Go Mono lacks).
type fontBundle struct {
	face     font.Face
	fallback font.Face
	cellW    int
	cellH    int
	ascent   int
}

var (
	fontCandidatesOnce sync.Once
	fontCandidates     [][]byte
)

func systemMonoCandidates() [][]byte {
	fontCandidatesOnce.Do(func() {
		home, _ := os.UserHomeDir()
		// Prefer Medium/SemiBold — Regular greyscale AA looks thin/muddy vs Kitty.
		paths := []string{
			filepath.Join(home, "Library", "Fonts", "JetBrainsMono-Medium.ttf"),
			filepath.Join(home, "Library", "Fonts", "JetBrainsMono-SemiBold.ttf"),
			filepath.Join(home, "Library", "Fonts", "JetBrainsMonoNL-Medium.ttf"),
			filepath.Join(home, "Library", "Fonts", "JetBrainsMono-Regular.ttf"),
			filepath.Join(home, "Library", "Fonts", "JetBrainsMonoNL-Regular.ttf"),
			"/Library/Fonts/JetBrainsMono-Medium.ttf",
			"/Library/Fonts/JetBrainsMono-Regular.ttf",
			"/System/Library/Fonts/SFNSMono.ttf",
			"/System/Library/Fonts/Menlo.ttc",
		}
		for _, p := range paths {
			data, err := os.ReadFile(p) //nolint:gosec // G304: path is a hardcoded relative to user home dir
			if err != nil || len(data) == 0 {
				continue
			}
			fontCandidates = append(fontCandidates, data)
		}
		if g, err := opentype.Parse(gomono.TTF); err == nil {
			_ = g
			fontCandidates = append(fontCandidates, gomono.TTF)
		} else {
			fontCandidates = append(fontCandidates, gomono.TTF)
		}
	})
	return fontCandidates
}

func openFace(data []byte, size, dpi float64) (font.Face, error) {
	// TTC collections (Menlo.ttc).
	if col, err := opentype.ParseCollection(data); err == nil && col.NumFonts() > 0 {
		fnt, err := col.Font(0)
		if err != nil {
			return nil, err
		}
		return opentype.NewFace(fnt, &opentype.FaceOptions{
			Size: size,
			DPI:  dpi,
			// None: sharper on Retina; Full greyscale AA looks mushy vs Kitty GPU AA.
			Hinting: font.HintingNone,
		})
	}
	fnt, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(fnt, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingNone,
	})
}

func faceHasRune(face font.Face, r rune) bool {
	if face == nil {
		return false
	}
	adv, ok := face.GlyphAdvance(r)
	return ok && adv > 0
}

func loadFontBundle(size, scaleFactor float64) fontBundle {
	dpi := 72.0 * scaleFactor
	var primary font.Face

	for _, data := range systemMonoCandidates() {
		f, err := openFace(data, size, dpi)
		if err != nil || f == nil {
			continue
		}
		primary = f
		break
	}
	if primary == nil {
		f, err := openFace(gomono.TTF, size, dpi)
		if err != nil {
			panic("gogpu: failed to load any monospace font: " + err.Error())
		}
		primary = f
	}

	m := primary.Metrics()
	b := fontBundle{face: primary, cellH: m.Height.Ceil(), ascent: m.Ascent.Ceil()}
	if adv, ok := primary.GlyphAdvance('M'); ok {
		b.cellW = adv.Ceil()
	} else {
		b.cellW = b.cellH / 2
	}
	// If primary lacks prompt arrows, keep SF Mono / second candidate as fallback.
	if !faceHasRune(primary, '→') && !faceHasRune(primary, '❯') && !faceHasRune(primary, '➜') {
		for _, data := range systemMonoCandidates() {
			f, err := openFace(data, size, dpi)
			if err != nil || f == nil || f == primary {
				continue
			}
			if faceHasRune(f, '→') || faceHasRune(f, '❯') || faceHasRune(f, '➜') {
				b.fallback = f
				break
			}
		}
	}
	return b
}
