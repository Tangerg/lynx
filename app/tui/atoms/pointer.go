package atoms

import (
	"image"

	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// Pointer tracks the mouse across the frames of one interface.
//
// A mouse event says where the pointer is; it does not say what is there. Working that
// out is layout's business, and layout only exists while a frame is being drawn — so a
// widget learns it was clicked by claiming the region it drew itself into, and asking.
//
// # Why a press is remembered
//
// A button that fired on the way down fires when the user was aiming at it and changed
// their mind. Every interface people already use commits on release, over the same
// target that took the press, which means something has to remember which target that
// was between two events. That is what this type is for, and it is why hover and press
// cannot be answered by a widget looking at one event on its own.
//
// It belongs to the loop that draws, like everything else at this layer, and holds no
// lock.
type Pointer struct {
	// at is where the pointer is, and inside says whether it is anywhere at all: a
	// pointer that has never been reported is not at the origin, it is nowhere.
	at     image.Point
	inside bool

	// captured is the region that took the press, and holding says whether one is being
	// held. A release outside it is not a click on it.
	captured image.Rectangle
	holding  bool
	button   input.Button
}

// Handle takes a mouse event, reporting whether it was one.
//
// Everything else is left alone: a pointer that consumed keys would be a pointer that
// swallowed typing.
func (p *Pointer) Handle(ev input.Event) bool {
	mouse, ok := ev.(input.Mouse)
	if !ok {
		return false
	}
	p.at, p.inside = mouse.Pos, true
	switch mouse.Action {
	case input.MouseDown:
		// The region is not known yet — nothing has been drawn since this event. It is
		// filled in by whichever widget claims the press on the next frame.
		p.holding, p.button, p.captured = true, mouse.Button, image.Rectangle{}
	case input.MouseUp:
		p.holding = false
		// The gesture is over, and whether it was a click was decided here: a release
		// somewhere other than where the press landed is how a user takes back a press
		// they did not mean. Deciding it now rather than a frame later keeps the answer
		// where the evidence is.
		if !p.captured.Empty() && !mouse.Pos.In(p.captured) {
			p.captured = image.Rectangle{}
		}
	}
	return true
}

// Left reports that the pointer is no longer over the interface, so nothing is hovered.
// A terminal does not report the mouse leaving, but a window losing focus is as close as
// it gets and is worth honouring: a hover left highlighted under an unfocused window
// looks like the interface is still live.
func (p *Pointer) Left() {
	p.inside = false
	p.holding = false
}

// Position is where the pointer is, and whether it is anywhere.
func (p *Pointer) Position() (image.Point, bool) { return p.at, p.inside }

// Over reports whether the pointer is inside a region, in the coordinates of whatever
// drew it. A widget passes the box it is drawing into.
func (p *Pointer) Over(region image.Rectangle) bool {
	return p.inside && p.at.In(region)
}

// Pressing reports whether a press is being held over a region, which is what draws a
// control as pushed in.
//
// It follows the press rather than the pointer: dragging off a button and back again
// keeps it pushed, because the press was never released.
func (p *Pointer) Pressing(region image.Rectangle) bool {
	if !p.holding {
		return false
	}
	if p.captured.Empty() {
		// The press has not been claimed yet. Whoever is under it claims it now, which is
		// how the region gets recorded in the first place.
		return p.at.In(region)
	}
	return p.captured == region
}

// Claim records that a region owns the press being held, if one is unclaimed and landed
// inside it. It is called while drawing, by the widget that drew the region.
func (p *Pointer) Claim(region image.Rectangle) {
	if p.holding && p.captured.Empty() && p.at.In(region) {
		p.captured = region
	}
}

// Clicked reports that a press taken by a region has been released over it, and takes
// the click so nothing else can answer the same one.
//
// Taken, rather than reported repeatedly: a click is an event, and a widget asking twice
// in one frame — or two widgets asking in turn — must not both act on it.
func (p *Pointer) Clicked(region image.Rectangle, button input.Button) bool {
	if p.holding || p.captured.Empty() || p.captured != region {
		return false
	}
	if p.button != button {
		return false
	}
	p.captured = image.Rectangle{}
	return true
}
