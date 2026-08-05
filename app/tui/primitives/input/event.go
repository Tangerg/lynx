// Package input turns the bytes a terminal sends into events.
//
// A terminal reports input as a stream that mixes plain text with escape
// sequences, and it splits that stream wherever the read happens to land: half a
// sequence in one read and half in the next is normal, not an error. [Parser] is
// therefore incremental — it is fed whatever arrived and returns whatever is now
// unambiguous.
//
// Nothing here touches a terminal. The parser is a function of its bytes, which
// is what lets every sequence this package claims to understand be stated as a
// test.
package input

import (
	"image"
	"strconv"
	"strings"
)

// Event is one thing the terminal reported. The set is closed by the unexported
// method: a consumer's switch over events is exhaustive by construction.
type Event interface {
	terminalEvent()
}

// Mods is the set of modifier keys held during an event.
type Mods uint8

const (
	Shift Mods = 1 << iota
	Alt
	Ctrl
	// Super is the platform's own modifier — Command on macOS, the Windows key
	// elsewhere. Only terminals speaking the Kitty keyboard protocol report it.
	Super
)

// Has reports whether every modifier in want is held.
func (m Mods) Has(want Mods) bool { return m&want == want }

// String names the modifiers in the order a keybinding is conventionally written.
func (m Mods) String() string {
	var parts []string
	for _, named := range []struct {
		mod  Mods
		name string
	}{{Ctrl, "ctrl"}, {Alt, "alt"}, {Shift, "shift"}, {Super, "super"}} {
		if m.Has(named.mod) {
			parts = append(parts, named.name)
		}
	}
	return strings.Join(parts, "+")
}

// Code identifies which key was pressed. [Character] means the key produced text,
// carried in [Key.Rune].
type Code int

const (
	// Character is the zero value, so a Key literal with only a rune in it is a
	// character press — which is what most of them are.
	Character Code = iota
	Enter
	Esc
	Backspace
	Tab
	Backtab
	Up
	Down
	Left
	Right
	Home
	End
	PageUp
	PageDown
	Delete
	Insert
	F1
	F2
	F3
	F4
	F5
	F6
	F7
	F8
	F9
	F10
	F11
	F12
)

// String names the key the way a help line would print it.
func (c Code) String() string {
	if name, ok := codeNames[c]; ok {
		return name
	}
	if c >= F1 && c <= F12 {
		return "f" + strconv.Itoa(int(c-F1)+1)
	}
	return "unknown"
}

var codeNames = map[Code]string{
	Character: "char",
	Enter:     "enter",
	Esc:       "esc",
	Backspace: "backspace",
	Tab:       "tab",
	Backtab:   "shift+tab",
	Up:        "up",
	Down:      "down",
	Left:      "left",
	Right:     "right",
	Home:      "home",
	End:       "end",
	PageUp:    "pageup",
	PageDown:  "pagedown",
	Delete:    "delete",
	Insert:    "insert",
}

// Transition is what happened to a key.
type Transition uint8

const (
	// Press is the zero value: an ordinary terminal only ever reports presses,
	// and a Key literal that says nothing about its transition means one.
	Press Transition = iota
	Repeat
	Release
)

// Key is a keyboard event.
//
// A character key arrives as [Character] with the rune in Rune. Ctrl held with a
// letter also arrives as a character — the letter, lowercased, with [Ctrl] in
// Mods — because that is what the terminal actually sends and inventing a
// separate representation for it would mean two ways to ask the same question.
type Key struct {
	Code Code
	Rune rune
	Mods Mods
	// Transition is Press unless the terminal speaks the Kitty keyboard protocol,
	// which is the only way repeats and releases are ever reported.
	Transition Transition
	// Text is what the key produced, when the terminal was able to say. It can
	// hold more than one code point, and is empty on terminals that do not report
	// it — Rune is the fallback and the common case.
	Text string
}

func (Key) terminalEvent() {}

// Is reports whether the key is code with exactly mods held.
//
// Exactly, not at least: a binding on Ctrl+C that also fired for Ctrl+Shift+C
// would swallow a keystroke its owner never claimed.
func (k Key) Is(code Code, mods Mods) bool {
	return k.Code == code && k.Mods == mods
}

// IsRune reports whether the key is the character r with exactly mods held.
func (k Key) IsRune(r rune, mods Mods) bool {
	return k.Code == Character && k.Rune == r && k.Mods == mods
}

// Down reports whether the key is going down — pressed or auto-repeating.
// Most handlers want this rather than Press alone, or holding a key stops working
// on terminals that report repeats.
func (k Key) Down() bool { return k.Transition != Release }

// String names the keystroke the way a help line or a keybinding file writes it.
func (k Key) String() string {
	var b strings.Builder
	if mods := k.Mods.String(); mods != "" {
		b.WriteString(mods)
		b.WriteByte('+')
	}
	if k.Code == Character {
		if k.Rune == ' ' {
			b.WriteString("space")
		} else {
			b.WriteRune(k.Rune)
		}
		return b.String()
	}
	b.WriteString(k.Code.String())
	return b.String()
}

// MouseAction is what the mouse did.
type MouseAction uint8

const (
	MouseDown MouseAction = iota
	MouseUp
	MouseDrag
	MouseMove
	WheelUp
	WheelDown
)

// Button identifies which mouse button an action belongs to.
type Button uint8

const (
	// ButtonNone is the zero value, which is right for a bare move and for a
	// wheel: neither belongs to a button.
	ButtonNone Button = iota
	ButtonLeft
	ButtonMiddle
	ButtonRight
)

// Mouse is a mouse event, positioned in cells with the origin at the top left.
type Mouse struct {
	Pos    image.Point
	Action MouseAction
	Button Button
	Mods   Mods
}

func (Mouse) terminalEvent() {}

// Paste is a block of text the terminal delivered as a paste rather than as
// keystrokes, so it can be inserted whole instead of being interpreted a
// character at a time.
type Paste struct{ Text string }

func (Paste) terminalEvent() {}

// Resize reports the terminal's new size in cells.
type Resize struct{ Width, Height int }

func (Resize) terminalEvent() {}

// FocusIn reports that the terminal window took focus.
type FocusIn struct{}

func (FocusIn) terminalEvent() {}

// FocusOut reports that the terminal window lost focus.
type FocusOut struct{}

func (FocusOut) terminalEvent() {}
