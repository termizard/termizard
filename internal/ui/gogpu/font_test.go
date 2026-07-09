package gogpu

import (
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func TestOpenFaceGoMono(t *testing.T) {
	face, err := openFace(gomono.TTF, 14, 72)
	if err != nil {
		t.Fatalf("openFace: %v", err)
	}
	if face == nil {
		t.Fatal("face is nil")
	}
	m := face.Metrics()
	if m.Height.Ceil() <= 0 {
		t.Fatal("zero height")
	}
}

func TestFaceHasRune(t *testing.T) {
	face, err := openFace(gomono.TTF, 14, 72)
	if err != nil {
		t.Fatal(err)
	}
	if !faceHasRune(face, 'M') {
		t.Fatal("M should be in Go Mono")
	}
	if faceHasRune(nil, 'M') {
		t.Fatal("nil face")
	}
}

func TestLoadFontBundle(t *testing.T) {
	b := loadFontBundle(14, 2.0)
	if b.face == nil {
		t.Fatal("primary face nil")
	}
	if b.cellW <= 0 || b.cellH <= 0 {
		t.Fatalf("cell %dx%d", b.cellW, b.cellH)
	}
	if b.ascent <= 0 {
		t.Fatal("ascent zero")
	}
}

func TestSystemMonoCandidatesIncludesGoMono(t *testing.T) {
	cands := systemMonoCandidates()
	if len(cands) == 0 {
		t.Fatal("no font candidates")
	}
	found := false
	for _, c := range cands {
		if len(c) == len(gomono.TTF) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("gomono.TTF not in candidates")
	}
}
