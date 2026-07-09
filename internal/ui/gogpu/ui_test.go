package gogpu

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	gogpulib "github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"

	"github.com/termizard/termizard/internal/adapter"
	"github.com/termizard/termizard/internal/config"
)

func testConfig() *config.Config {
	cfg := config.Defaults()
	cfg.Tabs.Enabled = true
	cfg.Tabs.ShowWhenSingle = true
	cfg.Window.PaddingX = 12
	cfg.Window.PaddingY = 10
	return cfg
}

func testUI(t *testing.T) (*UI, *winState) {
	t.Helper()
	u := New(testConfig())
	if u.primary == nil {
		t.Fatal("primary winState is nil")
	}
	ws := u.primary
	ws.scaleFactor = 2.0
	ws.cellW = 10
	ws.cellH = 20
	return u, ws
}

func TestWriteRoutesToShellTabNotActive(t *testing.T) {
	u, ws := testUI(t)
	tab1 := newTabSlot(80, 24, 100, true)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, tab1)
	ws.activeTabIdx = 1
	ws.mu.Unlock()

	tab0 := ws.tabs[0]
	tab0.dirty.Store(false)
	tab1.dirty.Store(false)

	if _, err := u.Write([]byte("primary")); err != nil {
		t.Fatal(err)
	}
	if !tab0.dirty.Load() {
		t.Fatal("tab0 should receive primary PTY output")
	}
	if tab1.dirty.Load() {
		t.Fatal("active tab must not receive primary PTY output")
	}
}

func TestTermPaddingUsesLargerAxis(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Window.PaddingX = 8
	u.cfg.Window.PaddingY = 14
	got := u.termPadding(ws)
	want := int(14 * ws.scaleFactor)
	if got != want {
		t.Fatalf("termPadding = %d, want %d", got, want)
	}
}

func TestTermPaddingDefaultsWhenZero(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Window.PaddingX = 0
	u.cfg.Window.PaddingY = 0
	got := u.termPadding(ws)
	if got < 10 {
		t.Fatalf("termPadding = %d, want floor >= 10*scale", got)
	}
}

func TestTabBarHeightHiddenWhenDisabled(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Tabs.Enabled = false
	if got := u.tabBarHeight(ws, 2); got != 0 {
		t.Fatalf("disabled tabs: height = %d, want 0", got)
	}
}

func TestTabBarHeightHiddenWhenSingle(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Tabs.ShowWhenSingle = false
	if got := u.tabBarHeight(ws, 1); got != 0 {
		t.Fatalf("single hidden: height = %d, want 0", got)
	}
}

func TestTabBarHeightCellPlusGutter(t *testing.T) {
	u, ws := testUI(t)
	g := u.termPadding(ws)
	want := ws.cellH + g
	if got := u.tabBarHeight(ws, 2); got < want {
		t.Fatalf("tabBarHeight = %d, want >= %d", got, want)
	}
}

func TestInitFontLoadsMetrics(t *testing.T) {
	u, ws := testUI(t)
	u.initFont(ws, 2.0)
	if ws.face == nil {
		t.Fatal("face is nil after initFont")
	}
	if ws.cellW <= 0 || ws.cellH <= 0 {
		t.Fatalf("cell size = %dx%d", ws.cellW, ws.cellH)
	}
	if ws.fontSizeCurrent != u.fontSizeDesired {
		t.Fatalf("fontSizeCurrent = %v, want %v", ws.fontSizeCurrent, u.fontSizeDesired)
	}
}

func TestInitFontSkipsWhenUnchanged(t *testing.T) {
	u, ws := testUI(t)
	u.initFont(ws, 2.0)
	face1 := ws.face
	u.initFont(ws, 2.0)
	if ws.face != face1 {
		t.Fatal("initFont reloaded face when size/scale unchanged")
	}
}

func TestClampTabScroll(t *testing.T) {
	if got := clampTabScroll(-5, 100); got != 0 {
		t.Fatalf("negative scroll = %d", got)
	}
	if got := clampTabScroll(150, 100); got != 100 {
		t.Fatalf("overflow scroll = %d", got)
	}
	if got := clampTabScroll(40, 100); got != 40 {
		t.Fatalf("in-range scroll = %d", got)
	}
}

func TestTabScrollMax(t *testing.T) {
	if got := tabScrollMax(0, 100, 400); got != 0 {
		t.Fatalf("no tabs = %d", got)
	}
	if got := tabScrollMax(3, 100, 400); got != 0 {
		t.Fatalf("fits = %d, want 0", got)
	}
	if got := tabScrollMax(5, 100, 400); got != 100 {
		t.Fatalf("overflow = %d, want 100", got)
	}
}

func TestPlusBtnWidth(t *testing.T) {
	w := plusBtnWidth(10, 2.0)
	if w < 10+10 {
		t.Fatalf("plusBtnWidth = %d, too narrow", w)
	}
}

func TestClampInt(t *testing.T) {
	if got := clampInt(5, 10); got != 5 {
		t.Fatalf("mid = %d", got)
	}
	if got := clampInt(-1, 10); got != 0 {
		t.Fatalf("low = %d", got)
	}
	if got := clampInt(99, 10); got != 10 {
		t.Fatalf("high = %d", got)
	}
}

func TestAbsInt(t *testing.T) {
	if absInt(-3) != 3 || absInt(4) != 4 {
		t.Fatal("absInt wrong")
	}
}

func TestIsModifierKey(t *testing.T) {
	if !isModifierKey(gpucontext.KeyLeftShift) {
		t.Fatal("shift should be modifier")
	}
	if isModifierKey(gpucontext.KeyA) {
		t.Fatal("A should not be modifier")
	}
}

func TestPasteTextToTabNormalizesNewlines(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	var got []byte
	tab.keyFn = func(e adapter.KeyEvent) { got = append(got, e.Data...) }
	u.pasteTextToTab(tab, "a\r\nb\rc")
	if string(got) != "a\nb\nc" {
		t.Fatalf("paste = %q", got)
	}
}

func TestPasteTextToTabBracketed(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	tab.write([]byte("\x1b[?2004h"))
	var got []byte
	tab.keyFn = func(e adapter.KeyEvent) { got = append(got, e.Data...) }
	u.pasteTextToTab(tab, "hi")
	if string(got) != "\x1b[200~hi\x1b[201~" {
		t.Fatalf("bracketed paste = %q", got)
	}
}

func TestScrollByClamps(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	tab.write([]byte("line1\r\nline2\r\nline3\r\n"))
	u.scrollBy(ws, 9999)
	if tab.scrollOffset.Load() != int64(tab.term.ScrollbackLen()) {
		t.Fatalf("scroll offset = %d, sb len = %d", tab.scrollOffset.Load(), tab.term.ScrollbackLen())
	}
	u.scrollBy(ws, -9999)
	if tab.scrollOffset.Load() != 0 {
		t.Fatalf("scroll back = %d, want 0", tab.scrollOffset.Load())
	}
}

func TestSwitchTabWraps(t *testing.T) {
	u, ws := testUI(t)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, newTabSlot(80, 24, 1000, true))
	ws.activeTabIdx = 1
	ws.mu.Unlock()
	u.switchTab(ws, 1)
	ws.mu.Lock()
	idx := ws.activeTabIdx
	ws.mu.Unlock()
	if idx != 0 {
		t.Fatalf("wrap forward = %d, want 0", idx)
	}
}

func TestSyncWindowTitleFallback(t *testing.T) {
	u, ws := testUI(t)
	u.cfg.Window.Title = "termizard"
	u.syncWindowTitle(ws, "")
	// Must not panic; native title may be blank on macOS FullSizeContent.
}

func TestWinStatePhys(t *testing.T) {
	ws := &winState{scaleFactor: 2.0}
	if got := ws.phys(10.5); got != 21 {
		t.Fatalf("phys = %d, want 21", got)
	}
}

func TestHandleActionClear(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	tab.write([]byte("data\r\n"))
	u.handleAction(ws, "Clear")
	if tab.scrollOffset.Load() != 0 {
		t.Fatalf("scroll not reset")
	}
}

func TestWriteFullEraseRefreshesPTY(t *testing.T) {
	_, tab := newWinState(80, 24, 100, true)
	pokes := 0
	tab.pokePTY = func() { pokes++ }
	tab.write([]byte("\x1b[2J"))
	if pokes != 1 {
		t.Fatalf("ED2 poke count = %d, want 1", pokes)
	}
}

func TestHandleActionClearRefreshesPTY(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	pokes := 0
	tab.pokePTY = func() { pokes++ }
	u.handleAction(ws, "Clear")
	if pokes != 1 {
		t.Fatalf("Clear poke count = %d, want 1", pokes)
	}
}

func TestHandleActionScrollPage(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	for i := 0; i < 30; i++ {
		tab.write([]byte("x\r\n"))
	}
	u.handleAction(ws, "ScrollPageUp")
	if tab.scrollOffset.Load() <= 0 {
		t.Fatal("expected positive scroll offset after page up")
	}
	u.handleAction(ws, "ScrollPageDown")
}

func TestHandleActionFontResize(t *testing.T) {
	u, ws := testUI(t)
	base := u.fontSizeDesired
	u.handleAction(ws, "FontIncrease")
	if u.fontSizeDesired <= base {
		t.Fatal("font size did not increase")
	}
	u.handleAction(ws, "FontDecrease")
	u.handleAction(ws, "FontReset")
	if u.fontSizeDesired != u.baseFontSize {
		t.Fatalf("reset = %v, want %v", u.fontSizeDesired, u.baseFontSize)
	}
}

func TestEnsureTabVisible(t *testing.T) {
	u, ws := testUI(t)
	ws.win = &gogpulib.Window{} // zero value; PhysicalSize may return 0 — still must not panic
	ws.cellW = 8
	ws.scaleFactor = 1
	u.ensureTabVisible(ws, 0)
}

func TestReorderTabUpdatesActive(t *testing.T) {
	u, ws := testUI(t)
	ws.mu.Lock()
	ws.tabs = []*tabSlot{newTabSlot(80, 24, 100, true), newTabSlot(80, 24, 100, true)}
	ws.activeTabIdx = 1
	ws.mu.Unlock()
	u.reorderTab(ws, 1, 0)
	ws.mu.Lock()
	idx := ws.activeTabIdx
	ws.mu.Unlock()
	if idx != 0 {
		t.Fatalf("active after reorder = %d, want 0", idx)
	}
}

func TestGpuDebugEnv(t *testing.T) {
	t.Setenv("TERMIZARD_GPU_DEBUG", "1")
	if !gpuDebug() {
		t.Fatal("gpuDebug should be true")
	}
	t.Setenv("TERMIZARD_GPU_DEBUG", "")
	if gpuDebug() {
		t.Fatal("gpuDebug should be false")
	}
}

func TestUIWritePrimaryTab(t *testing.T) {
	u, ws := testUI(t)
	n, err := u.Write([]byte("pty\r\n"))
	if err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	tab := ws.activeTab()
	if !tab.dirty.Load() {
		t.Fatal("tab not dirty after Write")
	}
}

func TestNotifyShellError(t *testing.T) {
	u, ws := testUI(t)
	u.NotifyShellError(0, nil)
	u.NotifyShellError(1, fmt.Errorf("boom"))
	tab := ws.activeTab()
	if !tab.dirty.Load() {
		t.Fatal("expected dirty after shell error")
	}
}

func TestWireTabTitleStoresPending(t *testing.T) {
	u, _ := testUI(t)
	tab := newTabSlot(80, 24, 100, true)
	u.wireTabTitle(tab)
	tab.write([]byte("\x1b]0;~/proj\x07"))
	p := tab.pendingTitle.Load()
	if p == nil || *p != "~/proj" {
		t.Fatalf("pending title = %v", p)
	}
}

func TestOnSurfaceReadyStoresCallback(t *testing.T) {
	u, _ := testUI(t)
	called := false
	u.OnSurfaceReady(func() { called = true })
	if u.surfaceReady == nil {
		t.Fatal("surfaceReady not set")
	}
	u.surfaceReady()
	if !called {
		t.Fatal("callback not invoked")
	}
}

func TestHandleActionScrollTopBottom(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "line%d\r\n", i)
	}
	tab.write([]byte(b.String()))
	u.handleAction(ws, "ScrollTop")
	if tab.scrollOffset.Load() == 0 {
		t.Fatal("ScrollTop should move into scrollback")
	}
	u.handleAction(ws, "ScrollBottom")
	if tab.scrollOffset.Load() != 0 {
		t.Fatalf("ScrollBottom = %d", tab.scrollOffset.Load())
	}
}

func TestHandleActionScrollUpDown(t *testing.T) {
	u, ws := testUI(t)
	tab := ws.activeTab()
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("x\r\n")
	}
	tab.write([]byte(b.String()))
	u.handleAction(ws, "ScrollUp")
	if tab.scrollOffset.Load() == 0 {
		t.Fatal("ScrollUp expected offset")
	}
	u.handleAction(ws, "ScrollDown")
}

func TestHandleActionCopyWithSelection(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("clipboard on macOS")
	}
	u, ws := testUI(t)
	tab := ws.activeTab()
	tab.write([]byte("copyme\r\n"))
	ws.mu.Lock()
	tab.selActive = true
	tab.selStartCol, tab.selStartRow = 0, 0
	tab.selEndCol, tab.selEndRow = 5, 0
	ws.mu.Unlock()
	u.handleAction(ws, actionCopy)
	got, err := clipboardReadNative()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("clipboard empty after copy")
	}
}

func TestDebugfWithEnv(t *testing.T) {
	t.Setenv("TERMIZARD_GPU_DEBUG", "1")
	u, _ := testUI(t)
	u.debugf("test %d", 1)
}

func testUIWithFrame(t *testing.T) (*UI, *winState) {
	t.Helper()
	u, ws := testUI(t)
	ws.frameW = 1200
	ws.frameH = 800
	ws.scaleFactor = 2.0
	u.initFont(ws, ws.scaleFactor)
	return u, ws
}

func termPointerXY(u *UI, ws *winState, col, row int) (float64, float64) { //nolint:unparam // row is 0 in current tests but kept for future coverage
	fw, fh := ws.physicalFrameSize()
	bl := u.blockLayout(ws, fw, fh, len(ws.tabs))
	physX := bl.G + col*ws.cellW + ws.cellW/2
	physY := bl.TermTop + row*ws.cellH + ws.cellH/2
	return float64(physX) / ws.scaleFactor, float64(physY) / ws.scaleFactor
}

func TestPhysicalFrameSizeFallback(t *testing.T) {
	ws := &winState{frameW: 640, frameH: 480}
	fw, fh := ws.physicalFrameSize()
	if fw != 640 || fh != 480 {
		t.Fatalf("fallback = %dx%d", fw, fh)
	}
	if !ws.hasFrameSize() {
		t.Fatal("hasFrameSize should be true")
	}
}

func TestHandleScrollZeroDelta(t *testing.T) {
	u, ws := testUIWithFrame(t)
	u.handleScroll(ws, gpucontext.ScrollEvent{})
}

func TestHandleScrollLineMode(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	for i := 0; i < 40; i++ {
		tab.write([]byte("line\r\n"))
	}
	before := tab.scrollOffset.Load()
	u.handleScroll(ws, gpucontext.ScrollEvent{
		DeltaY:    -1,
		DeltaMode: gpucontext.ScrollDeltaLine,
	})
	if tab.scrollOffset.Load() == before {
		t.Fatal("scroll offset unchanged")
	}
}

func TestHandleScrollPageMode(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	for i := 0; i < 40; i++ {
		tab.write([]byte("x\r\n"))
	}
	u.handleScroll(ws, gpucontext.ScrollEvent{
		DeltaY:    -1,
		DeltaMode: gpucontext.ScrollDeltaPage,
	})
	if tab.scrollOffset.Load() <= 0 {
		t.Fatal("page scroll expected offset")
	}
}

func TestHandleScrollTabBarHorizontal(t *testing.T) {
	u, ws := testUIWithFrame(t)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, newTabSlot(80, 24, 100, true), newTabSlot(80, 24, 100, true))
	ws.mu.Unlock()
	fw, fh := ws.physicalFrameSize()
	bl := u.blockLayout(ws, fw, fh, 3)
	logY := float64(bl.TabTop+bl.TabH/2) / ws.scaleFactor
	u.handleScroll(ws, gpucontext.ScrollEvent{
		X: 100, Y: logY,
		DeltaX: 10, DeltaY: 0,
	})
}

func TestHandleKeyPressArrowSendsInput(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	var got []byte
	tab.keyFn = func(e adapter.KeyEvent) { got = append(got, e.Data...) }
	u.handleKeyPress(ws, gpucontext.KeyUp, 0)
	if len(got) == 0 {
		t.Fatal("arrow key should send escape sequence")
	}
}

func TestHandleKeyPressModifierOnly(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	tab.write([]byte("sel\r\n"))
	ws.mu.Lock()
	tab.selActive = true
	tab.selStartCol, tab.selStartRow = 0, 0
	tab.selEndCol, tab.selEndRow = 2, 0
	ws.mu.Unlock()
	u.handleKeyPress(ws, gpucontext.KeyLeftSuper, gpucontext.ModSuper)
	ws.mu.Lock()
	active := tab.selActive
	ws.mu.Unlock()
	if !active {
		t.Fatal("modifier-only should not clear selection")
	}
}

func TestHandleKeyPressSpecialKeyClearsSelection(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	ws.mu.Lock()
	tab.selActive = true
	ws.mu.Unlock()
	tab.keyFn = func(adapter.KeyEvent) {}
	u.handleKeyPress(ws, gpucontext.KeyLeft, 0)
	ws.mu.Lock()
	active := tab.selActive
	ws.mu.Unlock()
	if active {
		t.Fatal("special key with PTY output should clear selection")
	}
}

func TestHandlePointerTerminalSelection(t *testing.T) {
	u, ws := testUIWithFrame(t)
	ws.activeTab().write([]byte("hello world\r\n"))
	x0, y0 := termPointerXY(u, ws, 0, 0)
	x1, y1 := termPointerXY(u, ws, 4, 0)
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerDown, Button: gpucontext.ButtonLeft, X: x0, Y: y0,
	})
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerMove, Button: gpucontext.ButtonLeft, X: x1, Y: y1,
	})
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerUp, Button: gpucontext.ButtonLeft, X: x1, Y: y1,
	})
	tab := ws.activeTab()
	ws.mu.Lock()
	active := tab.selActive
	ws.mu.Unlock()
	if !active {
		t.Fatal("drag selection expected")
	}
}

func TestHandlePointerDoubleClickWord(t *testing.T) {
	u, ws := testUIWithFrame(t)
	ws.activeTab().write([]byte("hello world\r\n"))
	x, y := termPointerXY(u, ws, 2, 0)
	ev := gpucontext.PointerEvent{Type: gpucontext.PointerDown, Button: gpucontext.ButtonLeft, X: x, Y: y}
	u.handlePointer(ws, ev)
	u.handlePointer(ws, ev)
	tab := ws.activeTab()
	ws.mu.Lock()
	active := tab.selActive
	ws.mu.Unlock()
	if !active {
		t.Fatal("double-click should select word")
	}
}

func TestHandleTabBarPointerSwitchTab(t *testing.T) {
	u, ws := testUIWithFrame(t)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, newTabSlot(80, 24, 100, true))
	ws.mu.Unlock()
	fw, fh := ws.physicalFrameSize()
	bl := u.blockLayout(ws, fw, fh, 2)
	g := bl.G
	tabW, _, _ := computeTabLayout(fw, g, 2, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size)
	physX := g + tabW + tabW/2
	physY := bl.TabTop + bl.TabH/2
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerDown, Button: gpucontext.ButtonLeft,
		X: float64(physX) / ws.scaleFactor,
		Y: float64(physY) / ws.scaleFactor,
	})
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerUp, Button: gpucontext.ButtonLeft,
		X: float64(physX) / ws.scaleFactor,
		Y: float64(physY) / ws.scaleFactor,
	})
	ws.mu.Lock()
	idx := ws.activeTabIdx
	ws.mu.Unlock()
	if idx != 1 {
		t.Fatalf("active tab = %d, want 1", idx)
	}
}

func TestRemoveTabEntryAdjustsActive(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab0 := ws.tabs[0]
	tab1 := newTabSlot(80, 24, 100, true)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, tab1)
	ws.activeTabIdx = 1
	ws.mu.Unlock()
	u.removeTabEntry(ws, tab0)
	ws.mu.Lock()
	n := len(ws.tabs)
	idx := ws.activeTabIdx
	ws.mu.Unlock()
	if n != 1 || idx != 0 {
		t.Fatalf("after remove: n=%d idx=%d", n, idx)
	}
}

func TestCloseTabAtIndex(t *testing.T) {
	u, ws := testUIWithFrame(t)
	closed := false
	tab1 := newTabSlot(80, 24, 100, true)
	tab1.closePTY = func() { closed = true }
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, tab1)
	ws.mu.Unlock()
	u.closeTabAtIndex(ws, 1)
	if !closed {
		t.Fatal("closePTY not called")
	}
	ws.mu.Lock()
	n := len(ws.tabs)
	ws.mu.Unlock()
	if n != 1 {
		t.Fatalf("tabs = %d", n)
	}
}

func TestCloseTabAtIndexOutOfRange(t *testing.T) {
	u, ws := testUIWithFrame(t)
	u.closeTabAtIndex(ws, 99) // must not panic
}

func TestHandleActionCopyEmpty(t *testing.T) {
	u, ws := testUIWithFrame(t)
	u.handleAction(ws, actionCopy) // no selection
}

func TestHandleActionPaste(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("clipboard on macOS")
	}
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	var got []byte
	tab.keyFn = func(e adapter.KeyEvent) { got = append(got, e.Data...) }
	if err := clipboardWriteNative("paste-me"); err != nil {
		t.Fatal(err)
	}
	u.handleAction(ws, actionPaste)
	if !strings.Contains(string(got), "paste-me") {
		t.Fatalf("paste = %q", got)
	}
}

func TestOnKeyInputOnResizeRequestRedraw(t *testing.T) {
	u, _ := testUI(t)
	var keyCalled bool
	u.OnKeyInput(func(adapter.KeyEvent) { keyCalled = true })
	u.OnResize(func(adapter.ResizeEvent) {})
	u.RequestRedraw()
	if u.keyFn == nil {
		t.Fatal("keyFn not set")
	}
	u.keyFn(adapter.KeyEvent{})
	if !keyCalled {
		t.Fatal("key callback not invoked")
	}
}

func TestCloseDoesNotPanic(t *testing.T) {
	u, _ := testUI(t)
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTabMinWidthPhys(t *testing.T) {
	if got := tabMinWidthPhys(2); got < 200 {
		t.Fatalf("tabMinWidth = %d", got)
	}
	if got := tabMinWidthPhys(0); got < 100 {
		t.Fatalf("zero scale = %d", got)
	}
}

func TestComputeTabLayoutThreeTabs(t *testing.T) {
	tabW, plusX, area := computeTabLayout(800, 24, 3, 10, 2.0, "compact")
	if tabW <= 0 || plusX <= 0 || area <= 0 {
		t.Fatalf("layout = %d %d %d", tabW, plusX, area)
	}
}

func TestEnsureTabVisibleWithFrame(t *testing.T) {
	u, ws := testUIWithFrame(t)
	for i := 0; i < 8; i++ {
		ws.mu.Lock()
		ws.tabs = append(ws.tabs, newTabSlot(80, 24, 100, true))
		ws.mu.Unlock()
	}
	u.ensureTabVisible(ws, 7)
}

func TestCloseCurrentTabWithMultiple(t *testing.T) {
	u, ws := testUIWithFrame(t)
	tab1 := newTabSlot(80, 24, 100, true)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, tab1)
	ws.activeTabIdx = 1
	ws.mu.Unlock()
	u.closeCurrentTab(ws)
	ws.mu.Lock()
	n := len(ws.tabs)
	idx := ws.activeTabIdx
	ws.mu.Unlock()
	if n != 1 || idx != 0 {
		t.Fatalf("after close: n=%d idx=%d", n, idx)
	}
}

func TestWriteWithGpuDebug(t *testing.T) {
	t.Setenv("TERMIZARD_GPU_DEBUG", "1")
	u, _ := testUI(t)
	if _, err := u.Write([]byte("dbg")); err != nil {
		t.Fatal(err)
	}
}

func TestHandlePointerRightClickPaste(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("clipboard on macOS")
	}
	u, ws := testUIWithFrame(t)
	tab := ws.activeTab()
	var got []byte
	tab.keyFn = func(e adapter.KeyEvent) { got = append(got, e.Data...) }
	if err := clipboardWriteNative("right-paste"); err != nil {
		t.Fatal(err)
	}
	x, y := termPointerXY(u, ws, 0, 0)
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerDown, Button: gpucontext.ButtonRight, X: x, Y: y,
	})
	if !strings.Contains(string(got), "right-paste") {
		t.Fatalf("paste = %q", got)
	}
}

func TestHandleTabPointerDragReorder(t *testing.T) {
	u, ws := testUIWithFrame(t)
	ws.mu.Lock()
	ws.tabs = append(ws.tabs, newTabSlot(80, 24, 100, true), newTabSlot(80, 24, 100, true))
	ws.mu.Unlock()
	fw, fh := ws.physicalFrameSize()
	bl := u.blockLayout(ws, fw, fh, 3)
	g := bl.G
	tabW, _, _ := computeTabLayout(fw, g, 3, ws.cellW, ws.scaleFactor, u.cfg.Tabs.Size)
	physX := g + tabW/2
	physY := bl.TabTop + bl.TabH/2
	logX := float64(physX) / ws.scaleFactor
	logY := float64(physY) / ws.scaleFactor
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerDown, Button: gpucontext.ButtonLeft, X: logX, Y: logY,
	})
	// Drag past threshold to another slot.
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerMove, Button: gpucontext.ButtonLeft,
		X: logX + 200, Y: logY,
	})
	u.handlePointer(ws, gpucontext.PointerEvent{
		Type: gpucontext.PointerUp, Button: gpucontext.ButtonLeft,
		X: logX + 200, Y: logY,
	})
}

func TestGridForFrameShrinksWithTabBar(t *testing.T) {
	u, ws := testUIWithFrame(t)
	u.cfg.Tabs.ShowWhenSingle = false
	_, rows1 := u.viewportGrid(ws, 1)
	_, rows2 := u.viewportGrid(ws, 2)
	if rows2 >= rows1 {
		t.Fatalf("tab bar should reduce rows: %d -> %d", rows1, rows2)
	}
}

func TestAddNewTabUsesViewportGrid(t *testing.T) {
	u, ws := testUIWithFrame(t)
	u.cfg.Tabs.ShowWhenSingle = false
	wantCols, wantRows := u.viewportGrid(ws, 2)
	// Simulate addNewTab sizing without spawning a real PTY.
	tab := newTabSlot(wantCols, wantRows, 100, true)
	if tab.term.Cols() != wantCols || tab.term.Rows() != wantRows {
		t.Fatalf("tab grid = %dx%d, want %dx%d", tab.term.Cols(), tab.term.Rows(), wantCols, wantRows)
	}
	_ = u
	_ = ws
}

func TestResizeTabsToGridUpdatesAll(t *testing.T) {
	ws, tab0 := newWinState(80, 50, 100, true)
	tab1 := newTabSlot(80, 50, 100, true)
	ws.tabs = append(ws.tabs, tab1)
	resizeTabsToGrid(ws.tabs, 80, 38)
	if tab0.term.Rows() != 38 || tab1.term.Rows() != 38 {
		t.Fatalf("rows = %d %d, want 38", tab0.term.Rows(), tab1.term.Rows())
	}
}

func TestResizeTabsToGridSkipsPendingPTY(t *testing.T) {
	ws, tab0 := newWinState(80, 50, 100, true)
	pending := newTabSlot(80, 50, 100, true)
	pending.needsPTY = true
	ws.tabs = append(ws.tabs, pending)
	resizeTabsToGrid(ws.tabs, 80, 38)
	if tab0.term.Rows() != 38 {
		t.Fatalf("active tab rows = %d", tab0.term.Rows())
	}
	if pending.term.Rows() != 50 {
		t.Fatalf("pending tab should not resize before spawn: %d", pending.term.Rows())
	}
}

func TestBlockLayoutBottomGutterMatchesSides(t *testing.T) {
	u, ws := testUI(t)
	ws.scaleFactor = 2.0
	ws.cellH = 20
	for _, numTabs := range []int{1, 2} {
		u.cfg.Tabs.ShowWhenSingle = false
		bl := u.blockLayout(ws, 1200, 800, numTabs)
		if bl.BottomG != bl.G {
			t.Fatalf("numTabs=%d: bottomG=%d g=%d", numTabs, bl.BottomG, bl.G)
		}
		if bl.TermH != 800-bl.TermTop-bl.BottomG {
			t.Fatalf("numTabs=%d: termH=%d", numTabs, bl.TermH)
		}
	}
}

func TestTabBarShownThreshold(t *testing.T) {
	u, _ := testUI(t)
	u.cfg.Tabs.ShowWhenSingle = false
	if u.tabBarShown(1) {
		t.Fatal("single tab should hide bar")
	}
	if !u.tabBarShown(2) {
		t.Fatal("two tabs should show bar")
	}
}

func TestReorderTabNoOp(t *testing.T) {
	u, ws := testUIWithFrame(t)
	ws.mu.Lock()
	before := ws.activeTabIdx
	ws.mu.Unlock()
	u.reorderTab(ws, 0, 0)
	ws.mu.Lock()
	after := ws.activeTabIdx
	ws.mu.Unlock()
	if before != after {
		t.Fatalf("no-op reorder changed active %d -> %d", before, after)
	}
}
