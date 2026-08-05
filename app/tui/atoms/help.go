package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Binding is a key and what it does.
//
// It pairs the two on purpose. A keystroke handled in one place and described in
// another drift apart, and the version the user reads is the one that is wrong.
type Binding struct {
	// Key is the keystroke, as the parser reports it, so the hint and the handler
	// are talking about the same thing.
	Key input.Key
	// What it does, in as few words as fit.
	Does string
	// Hidden keeps a binding out of the hint row without making it any less real —
	// for a chord that works but that nobody needs told about.
	Hidden bool
}

// Matches reports whether ev is this binding's keystroke.
func (b Binding) Matches(ev input.Event) bool {
	key, ok := ev.(input.Key)
	return ok && key.Down() && key.Code == b.Key.Code &&
		key.Rune == b.Key.Rune && key.Mods == b.Key.Mods
}

// Help is a row of key hints.
//
// The keys come from the same [Binding] values the handlers match against, which is
// what stops the hints and the behaviour from disagreeing.
type Help struct {
	Bindings  []Binding
	KeyStyle  grid.Style
	DoesStyle grid.Style
	// Separator sits between hints. Empty uses two spaces, which separates without
	// adding another thing to look at.
	Separator      string
	SeparatorStyle grid.Style
}

// Height is one row.
func (h Help) Height(int) int { return 1 }

// Draw writes as many hints as fit, in order, dropping the rest.
//
// Dropping rather than truncating the last one: half a hint is not a hint, and the
// bindings are listed in the order they matter so the ones that survive a narrow
// terminal are the ones worth keeping.
func (h Help) Draw(v grid.View) {
	w, height := v.Size()
	if w <= 0 || height <= 0 {
		return
	}
	separator := h.Separator
	if separator == "" {
		separator = "  "
	}
	sepWidth := text.Width(separator)

	x := 0
	for _, b := range h.Bindings {
		if b.Hidden {
			continue
		}
		key := b.Key.String()
		hint := text.Width(key) + 1 + text.Width(b.Does)
		need := hint
		if x > 0 {
			need += sepWidth
		}
		if x+need > w {
			return
		}
		if x > 0 {
			x += v.Text(x, 0, separator, h.SeparatorStyle)
		}
		x += v.Text(x, 0, key, h.KeyStyle)
		x += v.Text(x, 0, " ", h.DoesStyle)
		x += v.Text(x, 0, b.Does, h.DoesStyle)
	}
}
