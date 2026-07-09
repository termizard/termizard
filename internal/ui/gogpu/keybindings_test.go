package gogpu

import (
	"image/color"
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/termizard/termizard/internal/config"
	"github.com/termizard/termizard/internal/core/terminal"
)

func TestMatchPlatformBindingMacPaste(t *testing.T) {
	action := matchPlatformBinding(gpucontext.KeyV, gpucontext.ModSuper, true)
	if action != actionPaste {
		t.Fatalf("matchPlatformBinding(Cmd+V) = %q, want Paste", action)
	}
}

func TestMatchPlatformBindingWindowsPaste(t *testing.T) {
	action := matchPlatformBinding(gpucontext.KeyV, gpucontext.ModControl, false)
	if action != actionPaste {
		t.Fatalf("matchPlatformBinding(Ctrl+V) = %q, want Paste", action)
	}
}

func TestMatchPlatformBindingIgnoresCapsLock(t *testing.T) {
	action := matchPlatformBinding(gpucontext.KeyC, gpucontext.ModSuper|gpucontext.ModCapsLock, true)
	if action != actionCopy {
		t.Fatalf("Cmd+C with CapsLock = %q, want Copy", action)
	}
}

func TestClipboardEqual(t *testing.T) {
	if !clipboardEqual("a\r\nb", "a\nb") {
		t.Fatal("expected newline-normalized equality")
	}
}

func TestReorderTabSlots(t *testing.T) {
	tabs := []*tabSlot{{}, {}, {}, {}}
	got := reorderTabSlots(tabs, 0, 2)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[2] != tabs[0] {
		t.Fatal("expected first tab at index 2")
	}

	got = reorderTabSlots(tabs, 3, 0)
	if got[0] != tabs[3] {
		t.Fatal("expected last tab at index 0")
	}
}

func TestVisualTabSlot(t *testing.T) {
	// Drag tab 0 toward slot 2: tabs 1,2 shift left.
	if got := visualTabSlot(1, 0, 2); got != 0 {
		t.Fatalf("visualTabSlot(1,0,2) = %d, want 0", got)
	}
	if got := visualTabSlot(2, 0, 2); got != 1 {
		t.Fatalf("visualTabSlot(2,0,2) = %d, want 1", got)
	}
	// Drag tab 3 toward slot 0: tabs 0,1,2 shift right.
	if got := visualTabSlot(0, 3, 0); got != 1 {
		t.Fatalf("visualTabSlot(0,3,0) = %d, want 1", got)
	}
}

func TestTabIndexAtX(t *testing.T) {
	const fw = 400
	const cellW = 8
	const numTabs = 4
	const inset = 0
	tabW, _, _ := computeTabLayout(fw, inset, numTabs, cellW, 1, "full")

	if got := tabIndexAtX(tabW/4, fw, inset, numTabs, cellW, 1, "full", 0, 0); got != 0 {
		t.Fatalf("left quarter = %d, want 0", got)
	}
	if got := tabIndexAtX(tabW+tabW/4, fw, inset, numTabs, cellW, 1, "full", 0, 0); got != 1 {
		t.Fatalf("second tab = %d, want 1", got)
	}
}

func TestActiveTabPinHit(t *testing.T) {
	const fw = 400
	const cellW = 8
	const numTabs = 8
	const inset = 0
	tabW, _, tabsArea := computeTabLayout(fw, inset, numTabs, cellW, 1, "compact")
	if tabsArea >= numTabs*tabW {
		t.Fatalf("expected overflow: tabsArea=%d total=%d", tabsArea, numTabs*tabW)
	}
	// Scroll so active tab 0 is cut on the left → pin-left; left edge hits active.
	scroll := tabW * 2
	pinL, pinR := activeTabPin(0, numTabs, tabW, tabsArea, scroll)
	if !pinL || pinR {
		t.Fatalf("pin = (%v,%v), want left", pinL, pinR)
	}
	if got := tabIndexAtX(4, fw, inset, numTabs, cellW, 1, "compact", scroll, 0); got != 0 {
		t.Fatalf("pinned left hit = %d, want 0", got)
	}
}

func TestComputeTabLayoutStretch(t *testing.T) {
	tabW, _, _ := computeTabLayout(800, 0, 2, 8, 1, "full")
	if tabW < 300 {
		t.Fatalf("tabW = %d, want stretch (~391)", tabW)
	}
}

func TestComputeTabLayoutCompact(t *testing.T) {
	tabW, plusX, tabsArea := computeTabLayout(1200, 12, 2, 8, 1, "compact")
	if tabW != 200 {
		t.Fatalf("compact tabW = %d, want 200", tabW)
	}
	if tabsArea >= 1200-24 {
		t.Fatalf("tabsArea should leave room for + button, got %d", tabsArea)
	}
	if plusX < 12 {
		t.Fatalf("plusX = %d, want inset-aligned (>=12)", plusX)
	}
}

func TestComputeTabLayoutInsetAlign(t *testing.T) {
	const inset = 20
	const fw = 1000
	const cellW = 10
	_, plusX, tabsArea := computeTabLayout(fw, inset, 3, cellW, 1, "compact")
	plusW := plusBtnWidth(cellW, 1)
	contentR := fw - inset
	plusZoneL := contentR - plusW
	if inset+tabsArea+plusW != contentR {
		t.Fatalf("row width = %d, want content right %d", inset+tabsArea+plusW, contentR)
	}
	if plusX < plusZoneL || plusX+cellW > contentR {
		t.Fatalf("+ out of zone: plusX=%d zone=[%d,%d]", plusX, plusZoneL, contentR)
	}
}

func TestShortenTabTitle(t *testing.T) {
	got := shortenTabTitle("/Users/vlad/Workspace/opensource/gogpu-projects/wgpu-local")
	if got != "…/gogpu-projects/wgpu-local" {
		t.Fatalf("shorten = %q", got)
	}
	if got := shortenTabTitle("caffeinate"); got != "caffeinate" {
		t.Fatalf("process title = %q", got)
	}
}

func TestFillRectNegativeClip(t *testing.T) {
	// Dragging a tab left produces negative x — must not panic.
	buf := make([]byte, 100*20*4)
	fillRect(buf, 100, -50, 0, 20, 20, color.RGBA{R: 1, G: 2, B: 3, A: 255})
}

// stringToKey

func TestStringToKeyLetters(t *testing.T) {
	cases := []struct {
		s    string
		want gpucontext.Key
	}{
		{"a", gpucontext.KeyA},
		{"A", gpucontext.KeyA},
		{"z", gpucontext.KeyZ},
		{"m", gpucontext.KeyM},
	}
	for _, tc := range cases {
		k, ok := stringToKey(tc.s)
		if !ok || k != tc.want {
			t.Fatalf("stringToKey(%q) = %v,%v want %v,true", tc.s, k, ok, tc.want)
		}
	}
}

func TestStringToKeyDigits(t *testing.T) {
	k, ok := stringToKey("0")
	if !ok || k != gpucontext.Key0 {
		t.Fatalf("stringToKey('0') = %v,%v", k, ok)
	}
	k, ok = stringToKey("9")
	if !ok || k != gpucontext.Key0+9 {
		t.Fatalf("stringToKey('9') = %v,%v", k, ok)
	}
}

func TestStringToKeySpecialChars(t *testing.T) {
	cases := []struct {
		s    string
		want gpucontext.Key
	}{
		{"-", gpucontext.KeyMinus},
		{"=", gpucontext.KeyEqual},
		{"+", gpucontext.KeyEqual},
		{"[", gpucontext.KeyLeftBracket},
		{"]", gpucontext.KeyRightBracket},
		{`\`, gpucontext.KeyBackslash},
		{";", gpucontext.KeySemicolon},
		{"'", gpucontext.KeyApostrophe},
		{"`", gpucontext.KeyGrave},
		{",", gpucontext.KeyComma},
		{".", gpucontext.KeyPeriod},
		{"/", gpucontext.KeySlash},
	}
	for _, tc := range cases {
		k, ok := stringToKey(tc.s)
		if !ok || k != tc.want {
			t.Fatalf("stringToKey(%q) = %v,%v want %v", tc.s, k, ok, tc.want)
		}
	}
}

func TestStringToKeyArrows(t *testing.T) {
	cases := []struct {
		s    string
		want gpucontext.Key
	}{
		{"up", gpucontext.KeyUp},
		{"ArrowUp", gpucontext.KeyUp},
		{"down", gpucontext.KeyDown},
		{"ArrowDown", gpucontext.KeyDown},
		{keyDirLeft, gpucontext.KeyLeft},
		{"ArrowLeft", gpucontext.KeyLeft},
		{keyDirRight, gpucontext.KeyRight},
		{"ArrowRight", gpucontext.KeyRight},
	}
	for _, tc := range cases {
		k, ok := stringToKey(tc.s)
		if !ok || k != tc.want {
			t.Fatalf("stringToKey(%q) = %v,%v", tc.s, k, ok)
		}
	}
}

func TestStringToKeyNavigation(t *testing.T) {
	cases := []struct {
		s    string
		want gpucontext.Key
	}{
		{"home", gpucontext.KeyHome},
		{"end", gpucontext.KeyEnd},
		{"pageup", gpucontext.KeyPageUp},
		{"pagedown", gpucontext.KeyPageDown},
		{"insert", gpucontext.KeyInsert},
		{"delete", gpucontext.KeyDelete},
		{"escape", gpucontext.KeyEscape},
		{"esc", gpucontext.KeyEscape},
		{"tab", gpucontext.KeyTab},
		{"backspace", gpucontext.KeyBackspace},
		{"enter", gpucontext.KeyEnter},
		{"return", gpucontext.KeyEnter},
		{"space", gpucontext.KeySpace},
	}
	for _, tc := range cases {
		k, ok := stringToKey(tc.s)
		if !ok || k != tc.want {
			t.Fatalf("stringToKey(%q) = %v,%v", tc.s, k, ok)
		}
	}
}

func TestStringToKeyFKeys(t *testing.T) {
	cases := []struct {
		s    string
		want gpucontext.Key
	}{
		{"f1", gpucontext.KeyF1},
		{"f6", gpucontext.KeyF6},
		{"f12", gpucontext.KeyF12},
	}
	for _, tc := range cases {
		k, ok := stringToKey(tc.s)
		if !ok || k != tc.want {
			t.Fatalf("stringToKey(%q) = %v,%v", tc.s, k, ok)
		}
	}
}

func TestStringToKeyUnknown(t *testing.T) {
	_, ok := stringToKey("notakey")
	if ok {
		t.Fatal("unknown key name should return false")
	}
}

func TestStringToKeyEmptyString(t *testing.T) {
	_, ok := stringToKey("")
	if ok {
		t.Fatal("empty string should return false")
	}
}

// modsFromStrings

func TestModsFromStringsCtrl(t *testing.T) {
	m := modsFromStrings([]string{modCtrl})
	if m&gpucontext.ModControl == 0 {
		t.Fatal("ctrl not set")
	}
}

func TestModsFromStringsControlAlias(t *testing.T) {
	m := modsFromStrings([]string{modControl})
	if m&gpucontext.ModControl == 0 {
		t.Fatal("control alias not set")
	}
}

func TestModsFromStringsShift(t *testing.T) {
	m := modsFromStrings([]string{modShift})
	if m&gpucontext.ModShift == 0 {
		t.Fatal("shift not set")
	}
}

func TestModsFromStringsAlt(t *testing.T) {
	for _, name := range []string{modAlt, modOption} {
		m := modsFromStrings([]string{name})
		if m&gpucontext.ModAlt == 0 {
			t.Fatalf("alt alias %q not set", name)
		}
	}
}

func TestModsFromStringsMeta(t *testing.T) {
	for _, name := range []string{modMeta, modCmd, modSuper} {
		m := modsFromStrings([]string{name})
		if m&gpucontext.ModSuper == 0 {
			t.Fatalf("super alias %q not set", name)
		}
	}
}

func TestModsFromStringsMultiple(t *testing.T) {
	m := modsFromStrings([]string{modCtrl, modShift, modAlt})
	if m&gpucontext.ModControl == 0 || m&gpucontext.ModShift == 0 || m&gpucontext.ModAlt == 0 {
		t.Fatalf("multiple mods not set: %v", m)
	}
}

func TestModsFromStringsUnknown(t *testing.T) {
	m := modsFromStrings([]string{"unknown"})
	if m != 0 {
		t.Fatalf("unknown mod should yield 0, got %v", m)
	}
}

func TestModsFromStringsEmpty(t *testing.T) {
	m := modsFromStrings(nil)
	if m != 0 {
		t.Fatalf("empty list should yield 0, got %v", m)
	}
}

// compileBindings / matchBinding

func TestCompileBindingsValidEntry(t *testing.T) {
	kbs := []config.Keybinding{
		{Key: "c", Mods: []string{modCtrl}, Action: "Copy"},
	}
	cbs := compileBindings(kbs)
	if len(cbs) != 1 {
		t.Fatalf("expected 1 compiled binding, got %d", len(cbs))
	}
	if cbs[0].action != "Copy" {
		t.Fatalf("action = %q", cbs[0].action)
	}
}

func TestCompileBindingsSkipsUnknownKey(t *testing.T) {
	kbs := []config.Keybinding{
		{Key: "notakey", Mods: []string{"ctrl"}, Action: "Foo"},
	}
	cbs := compileBindings(kbs)
	if len(cbs) != 0 {
		t.Fatalf("expected 0 compiled bindings for unknown key, got %d", len(cbs))
	}
}

func TestMatchBindingHit(t *testing.T) {
	cbs := []compiledBinding{
		{key: gpucontext.KeyC, mods: gpucontext.ModControl, action: "Copy"},
	}
	action := matchBinding(gpucontext.KeyC, gpucontext.ModControl, cbs)
	if action != "Copy" {
		t.Fatalf("got %q, want Copy", action)
	}
}

func TestMatchBindingMiss(t *testing.T) {
	cbs := []compiledBinding{
		{key: gpucontext.KeyC, mods: gpucontext.ModControl, action: "Copy"},
	}
	action := matchBinding(gpucontext.KeyV, gpucontext.ModControl, cbs)
	if action != "" {
		t.Fatalf("expected no match, got %q", action)
	}
}

func TestMatchBindingStripsLockMods(t *testing.T) {
	cbs := []compiledBinding{
		{key: gpucontext.KeyC, mods: gpucontext.ModSuper, action: "Copy"},
	}
	action := matchBinding(gpucontext.KeyC, gpucontext.ModSuper|gpucontext.ModCapsLock|gpucontext.ModNumLock, cbs)
	if action != "Copy" {
		t.Fatalf("CapsLock+NumLock should be stripped: got %q", action)
	}
}

func TestMatchBindingEmptyList(t *testing.T) {
	action := matchBinding(gpucontext.KeyA, gpucontext.ModControl, nil)
	if action != "" {
		t.Fatalf("empty bindings should return empty string")
	}
}

// session helpers

func TestNewTabSlotNonNil(t *testing.T) {
	ts := newTabSlot(80, 24, 1000, true)
	if ts == nil || ts.term == nil || ts.parser == nil {
		t.Fatal("newTabSlot returned incomplete slot")
	}
}

func TestNewWinStateInitialState(t *testing.T) {
	ws, tab := newWinState(80, 24, 1000, true)
	if ws == nil || tab == nil {
		t.Fatal("newWinState returned nil")
	}
	if len(ws.tabs) != 1 || ws.tabs[0] != tab {
		t.Fatal("wrong initial tab")
	}
	if ws.activeTabIdx != 0 {
		t.Fatalf("activeTabIdx = %d, want 0", ws.activeTabIdx)
	}
	if ws.hoverTabIdx != -1 {
		t.Fatalf("hoverTabIdx = %d, want -1", ws.hoverTabIdx)
	}
	if !ws.blinkOn.Load() {
		t.Fatal("blinkOn should start true")
	}
}

func TestActiveTabValid(t *testing.T) {
	ws, tab := newWinState(80, 24, 100, false)
	if ws.activeTab() != tab {
		t.Fatal("activeTab should return the initial tab")
	}
}

func TestActiveTabEmpty(t *testing.T) {
	ws := &winState{activeTabIdx: 0}
	if ws.activeTab() != nil {
		t.Fatal("empty tabs should return nil")
	}
}

func TestSelNormalized(t *testing.T) {
	ts := &tabSlot{}
	ts.selStartCol, ts.selStartRow = 5, 3
	ts.selEndCol, ts.selEndRow = 2, 1
	c0, r0, c1, r1 := ts.selNormalized()
	if r0 > r1 {
		t.Fatalf("r0=%d > r1=%d after normalize", r0, r1)
	}
	if r0 != 1 || c0 != 2 || r1 != 3 || c1 != 5 {
		t.Fatalf("normalized = (%d,%d)-(%d,%d)", c0, r0, c1, r1)
	}
}

func TestSelNormalizedSameRow(t *testing.T) {
	ts := &tabSlot{}
	ts.selStartCol, ts.selStartRow = 8, 2
	ts.selEndCol, ts.selEndRow = 3, 2
	c0, r0, c1, r1 := ts.selNormalized()
	if c0 != 3 || c1 != 8 || r0 != 2 || r1 != 2 {
		t.Fatalf("same-row normalized = (%d,%d)-(%d,%d)", c0, r0, c1, r1)
	}
}

func TestComputeWordBounds(t *testing.T) {
	term := terminal.New(80, 24, 100, false)
	for _, r := range "hello world" {
		term.Print(r)
	}
	// "hello" occupies cols 0-4; col 2 is in the middle
	start, end := computeWordBounds(term, 2, 0)
	if start != 0 {
		t.Fatalf("word start = %d, want 0", start)
	}
	if end < 4 {
		t.Fatalf("word end = %d, want >=4", end)
	}
}

func TestComputeWordBoundsNonWord(t *testing.T) {
	term := terminal.New(80, 24, 100, false)
	// blank cell — non-word char should return (col, col)
	start, end := computeWordBounds(term, 10, 0)
	if start != 10 || end != 10 {
		t.Fatalf("non-word: start=%d end=%d", start, end)
	}
}

func TestResetTabPointer(t *testing.T) {
	ws := &winState{
		tabPressActive: true,
		tabPressIdx:    2,
		dragActive:     true,
		dragTabIdx:     1,
		dragDX:         50,
	}
	ws.resetTabPointer()
	if ws.tabPressActive || ws.dragActive || ws.dragDX != 0 {
		t.Fatal("resetTabPointer did not clear fields")
	}
}

// extractSelectedText with no active selection

func TestExtractSelectedTextNoSelection(t *testing.T) {
	ws, _ := newWinState(80, 24, 100, false)
	text := ws.extractSelectedText()
	if text != "" {
		t.Fatalf("no selection: expected empty, got %q", text)
	}
}

// activeTabPin edge cases

func TestActiveTabPinNoOverflow(t *testing.T) {
	pinL, pinR := activeTabPin(0, 3, 100, 500, 0)
	if pinL || pinR {
		t.Fatal("no overflow: should not pin")
	}
}

func TestActiveTabPinInvalid(t *testing.T) {
	pinL, pinR := activeTabPin(-1, 3, 100, 200, 0)
	if pinL || pinR {
		t.Fatal("invalid activeIdx should return false")
	}
}

func TestActiveTabPinZeroTabWidth(t *testing.T) {
	pinL, pinR := activeTabPin(0, 3, 0, 200, 0)
	if pinL || pinR {
		t.Fatal("zero tabW should return false")
	}
}
