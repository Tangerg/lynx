// Package atoms is the widget vocabulary: pieces that draw and respond, and mean
// nothing in particular.
//
// A list here is a list whether it holds sessions or files; a box is a box whether
// it frames a transcript or a warning. Nothing in this package knows what this
// program is for, which is what makes the pieces reusable and what makes their
// behaviour testable by stating it rather than by running the application.
//
// # How a widget works
//
// A widget is a mutable object owned by one loop. It is asked to draw itself into
// the space it was given, and asked whether it wants an event. It does not return a
// new copy of itself, and it does not know where on the screen it is: the view it
// is handed is already positioned and already clipped, so a widget's coordinates
// are its own and it cannot draw outside its box.
//
// Measurement is separate from drawing because a container has to know how tall its
// children want to be before it can decide where they go. A widget whose height
// follows from its width says so by implementing [Sized].
package atoms

import (
	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// Widget draws itself into the space it is given.
//
// The view is already positioned and clipped. A widget that draws outside it is
// not a bug that shows on screen — the drawing is simply discarded — which is what
// makes the box a boundary rather than a convention.
type Widget interface {
	Draw(v grid.View)
}

// Sized is a widget whose height follows from the width it is given: wrapped text,
// a list of variable-height rows, anything that reflows.
//
// Height is asked before Draw and must agree with it. A widget that reports one
// height and draws another gets clipped or leaves a gap, and both look like a
// layout bug somewhere else.
type Sized interface {
	Widget
	Height(width int) int
}

// Interactive is a widget that answers input.
//
// Handle reports whether the event was consumed. An unconsumed event carries on to
// whatever else might want it, which is how a key can mean one thing inside a text
// field and another outside it without either side knowing about the other.
type Interactive interface {
	Widget
	Handle(ev input.Event) bool
}

// Focusable is a widget that behaves differently when it has the keyboard.
//
// Focus is a container's to give: a widget can render itself as focused, but it
// cannot decide that it is.
type Focusable interface {
	Widget
	Focus(focused bool)
}

// Align is how content sits in a space wider than itself.
type Align uint8

const (
	Start Align = iota
	Center
	End
)

// offset is where content of the given width starts inside space columns.
func (a Align) offset(space, width int) int {
	switch a {
	case Center:
		return max((space-width)/2, 0)
	case End:
		return max(space-width, 0)
	default:
		return 0
	}
}
