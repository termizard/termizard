package gogpu

import (
	"strings"

	"github.com/gogpu/gpucontext"
	"github.com/termizard/termizard/internal/config"
)

// Modifier name strings used in config keybindings.
const (
	modCtrl    = "ctrl"
	modControl = "control"
	modShift   = "shift"
	modAlt     = "alt"
	modOption  = "option"
	modMeta    = "meta"
	modCmd     = "cmd"
	modSuper   = "super"
)

// Key direction name strings used in stringToKey.
const (
	keyDirLeft  = "left"
	keyDirRight = "right"
)

// compiledBinding is a pre-processed keybinding ready for O(n) matching.
type compiledBinding struct {
	key    gpucontext.Key
	mods   gpucontext.Modifiers
	action string
}

// compileBindings converts config keybindings to the fast match form.
// Unknown key names are silently skipped.
func compileBindings(bindings []config.Keybinding) []compiledBinding {
	out := make([]compiledBinding, 0, len(bindings))
	for _, kb := range bindings {
		k, ok := stringToKey(kb.Key)
		if !ok {
			continue
		}
		out = append(out, compiledBinding{
			key:    k,
			mods:   modsFromStrings(kb.Mods),
			action: kb.Action,
		})
	}
	return out
}

// matchBinding returns the action string for the first binding that matches key+mods,
// or "" if none match. CapsLock/NumLock are ignored for matching.
func matchBinding(key gpucontext.Key, mods gpucontext.Modifiers, bindings []compiledBinding) string {
	mods &^= gpucontext.ModCapsLock | gpucontext.ModNumLock
	for _, b := range bindings {
		if b.key == key && b.mods == mods {
			return b.action
		}
	}
	return ""
}

// platformBindings returns OS-specific shortcuts that mirror frontend/src/keybindings.ts.
// Config keybindings are checked separately and can extend but not override these.
func platformBindings(mac bool) []compiledBinding {
	if mac {
		meta := gpucontext.ModSuper
		return []compiledBinding{
			{key: gpucontext.KeyK, mods: meta, action: "Clear"},
			{key: gpucontext.KeyC, mods: meta, action: actionCopy},
			{key: gpucontext.KeyV, mods: meta, action: actionPaste},
			{key: gpucontext.KeyEqual, mods: meta, action: "FontIncrease"},
			{key: gpucontext.KeyMinus, mods: meta, action: "FontDecrease"},
			{key: gpucontext.Key0, mods: meta, action: "FontReset"},
		}
	}
	ctrl := gpucontext.ModControl
	shift := gpucontext.ModShift
	return []compiledBinding{
		{key: gpucontext.KeyInsert, mods: ctrl, action: actionCopy},
		{key: gpucontext.KeyInsert, mods: shift, action: actionPaste},
		{key: gpucontext.KeyV, mods: ctrl, action: actionPaste},
	}
}

// matchPlatformBinding returns a built-in platform shortcut action, or "" if none match.
func matchPlatformBinding(key gpucontext.Key, mods gpucontext.Modifiers, mac bool) string {
	return matchBinding(key, mods, platformBindings(mac))
}

// modsFromStrings converts a slice of modifier name strings to a Modifiers bitmask.
func modsFromStrings(names []string) gpucontext.Modifiers {
	var m gpucontext.Modifiers
	for _, n := range names {
		switch strings.ToLower(n) {
		case modCtrl, modControl:
			m |= gpucontext.ModControl
		case modShift:
			m |= gpucontext.ModShift
		case modAlt, modOption:
			m |= gpucontext.ModAlt
		case modMeta, modCmd, modSuper:
			m |= gpucontext.ModSuper
		}
	}
	return m
}

// stringToKey maps the key name strings used in config.Keybinding.Key to
// gpucontext.Key values. Names follow the convention used in keybindings.ts.
func stringToKey(s string) (gpucontext.Key, bool) {
	// Normalised single-char letter: "a".."z" (case-insensitive)
	lower := strings.ToLower(strings.TrimSpace(s))
	if len(lower) == 1 {
		ch := lower[0]
		if ch >= 'a' && ch <= 'z' {
			return gpucontext.KeyA + gpucontext.Key(ch-'a'), true
		}
		if ch >= '0' && ch <= '9' {
			return gpucontext.Key0 + gpucontext.Key(ch-'0'), true
		}
		switch ch {
		case '-':
			return gpucontext.KeyMinus, true
		case '=', '+': // '+' is shift+equal on US keyboards
			return gpucontext.KeyEqual, true
		case '[':
			return gpucontext.KeyLeftBracket, true
		case ']':
			return gpucontext.KeyRightBracket, true
		case '\\':
			return gpucontext.KeyBackslash, true
		case ';':
			return gpucontext.KeySemicolon, true
		case '\'':
			return gpucontext.KeyApostrophe, true
		case '`':
			return gpucontext.KeyGrave, true
		case ',':
			return gpucontext.KeyComma, true
		case '.':
			return gpucontext.KeyPeriod, true
		case '/':
			return gpucontext.KeySlash, true
		}
	}

	// Multi-character names (case-insensitive).
	switch lower {
	case "up", "arrowup":
		return gpucontext.KeyUp, true
	case "down", "arrowdown":
		return gpucontext.KeyDown, true
	case keyDirLeft, "arrowleft":
		return gpucontext.KeyLeft, true
	case keyDirRight, "arrowright":
		return gpucontext.KeyRight, true
	case "home":
		return gpucontext.KeyHome, true
	case "end":
		return gpucontext.KeyEnd, true
	case "pageup":
		return gpucontext.KeyPageUp, true
	case "pagedown":
		return gpucontext.KeyPageDown, true
	case "insert":
		return gpucontext.KeyInsert, true
	case "delete":
		return gpucontext.KeyDelete, true
	case "escape", "esc":
		return gpucontext.KeyEscape, true
	case "tab":
		return gpucontext.KeyTab, true
	case "backspace":
		return gpucontext.KeyBackspace, true
	case "enter", "return":
		return gpucontext.KeyEnter, true
	case "space":
		return gpucontext.KeySpace, true
	case "f1":
		return gpucontext.KeyF1, true
	case "f2":
		return gpucontext.KeyF2, true
	case "f3":
		return gpucontext.KeyF3, true
	case "f4":
		return gpucontext.KeyF4, true
	case "f5":
		return gpucontext.KeyF5, true
	case "f6":
		return gpucontext.KeyF6, true
	case "f7":
		return gpucontext.KeyF7, true
	case "f8":
		return gpucontext.KeyF8, true
	case "f9":
		return gpucontext.KeyF9, true
	case "f10":
		return gpucontext.KeyF10, true
	case "f11":
		return gpucontext.KeyF11, true
	case "f12":
		return gpucontext.KeyF12, true
	}
	return gpucontext.KeyUnknown, false
}
