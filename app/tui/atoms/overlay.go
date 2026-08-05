package atoms

import (
	"image"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
)

// Anchor is where a floating layer sits in the space it floats over.
type Anchor uint8

const (
	// Middle is the centre, which is where a modal belongs: it is the one position that
	// does not imply the thing it covers is still reachable.
	Middle Anchor = iota
	TopLeft
	Top
	TopRight
	Left
	Right
	BottomLeft
	Bottom
	BottomRight
)

// place works out where a box of size sits inside space.
func (a Anchor) place(space, size image.Point) image.Point {
	var at image.Point
	switch a {
	case TopLeft, Left, BottomLeft:
		at.X = 0
	case Top, Middle, Bottom:
		at.X = (space.X - size.X) / 2
	default:
		at.X = space.X - size.X
	}
	switch a {
	case TopLeft, Top, TopRight:
		at.Y = 0
	case Left, Middle, Right:
		at.Y = (space.Y - size.Y) / 2
	default:
		at.Y = space.Y - size.Y
	}
	return image.Pt(max(at.X, 0), max(at.Y, 0))
}

// Overlay floats one thing over another.
//
// It is placement, not a container: it works out where a layer goes and hands back the
// view to draw it in. What goes inside is the caller's, which is what lets the same
// overlay carry a dialog, a completion list, or a single line of warning.
//
// The layer is clamped to the space it floats over rather than allowed to hang off the
// edge. A dialog whose buttons are past the right margin is a dialog nobody can answer.
type Overlay struct {
	Anchor Anchor
	// Width and Height are the layer's size in cells. Zero means as large as the space
	// allows, less Margin.
	Width, Height int
	// Margin is kept clear between the layer and the edges of the space, so a layer
	// anchored to a corner does not look stuck to it.
	Margin int
	// Shade dims what the layer covers, so the eye goes to the layer and it is obvious
	// that what is behind it is not the thing to act on. Its zero value covers nothing.
	Shade grid.Style
}

// Draw dims what is behind the layer and returns the view to draw the layer into.
func (o Overlay) Draw(v grid.View) grid.View {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return v.Sub(image.Rectangle{})
	}
	if o.Shade != (grid.Style{}) {
		// Restyled rather than filled: what is behind stays legible and simply recedes,
		// which is what tells the reader it is still there and not gone.
		o.shade(v, width, height)
	}
	return v.Sub(o.Area(v))
}

// Area is where the layer goes, in the space's own coordinates. It is separate from
// drawing so that a hit test can ask the same question a frame later.
func (o Overlay) Area(v grid.View) image.Rectangle {
	width, height := v.Size()
	room := image.Pt(max(width-2*o.Margin, 0), max(height-2*o.Margin, 0))
	size := image.Pt(o.Width, o.Height)
	if size.X <= 0 || size.X > room.X {
		size.X = room.X
	}
	if size.Y <= 0 || size.Y > room.Y {
		size.Y = room.Y
	}
	at := o.Anchor.place(room, size).Add(image.Pt(o.Margin, o.Margin))
	return grid.Rect(at.X, at.Y, size.X, size.Y)
}

// shade restyles every cell of the space it covers.
func (o Overlay) shade(v grid.View, width, height int) {
	for y := range height {
		for x := range width {
			if cell := v.CellAt(x, y); cell != nil {
				cell.Style = cell.Style.Merge(o.Shade)
			}
		}
	}
}
