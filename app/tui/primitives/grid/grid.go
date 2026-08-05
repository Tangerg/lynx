// Package grid is the cell grid the whole terminal UI is drawn into: styled
// grapheme cells, a clipped drawing view over them, and a double-buffered screen
// that emits the smallest escape stream turning one frame into the next.
//
// It is the only layer that knows what a terminal is made of. Everything above
// it draws through [View] and never assembles an escape sequence.
//
// Geometry is [image.Rectangle] and [image.Point] from the standard library
// rather than a private rectangle type. Terminal rectangles are ordinary
// half-open rectangles, and intersection, insetting and containment are already
// written and already correct there.
package grid

import (
	"image"
	"math"
)

// Rect builds a rectangle from a terminal-natural origin and size. The result is
// half-open: it covers columns [x, x+w) and rows [y, y+h).
func Rect(x, y, w, h int) image.Rectangle {
	return image.Rect(x, y, x+w, y+h)
}

// RGB is a 24-bit colour.
type RGB struct{ R, G, B uint8 }

// Color is a cell colour: either the terminal's own default, or a truecolor
// value. The zero Color is the default, which is what an unstyled cell wants.
type Color struct {
	set bool
	rgb RGB
}

// RGBColor returns a colour that overrides the terminal default.
func RGBColor(r, g, b uint8) Color { return Color{set: true, rgb: RGB{r, g, b}} }

// Default reports whether the colour defers to the terminal.
func (c Color) Default() bool { return !c.set }

// RGB returns the colour's components. They are meaningless when the colour is
// the terminal default.
func (c Color) RGB() RGB { return c.rgb }

// Blend mixes c toward over by opacity, clamped to [0,1]. A blend involving the
// terminal default is over unchanged: there is no way to know what the default
// resolves to, and guessing would tint every theme differently.
func (c Color) Blend(over Color, opacity float64) Color {
	if c.Default() || over.Default() {
		return over
	}
	opacity = min(max(opacity, 0), 1)
	lerp := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*opacity))
	}
	return RGBColor(
		lerp(c.rgb.R, over.rgb.R),
		lerp(c.rgb.G, over.rgb.G),
		lerp(c.rgb.B, over.rgb.B),
	)
}

// Attr is a set of text attributes.
type Attr uint8

const (
	Bold Attr = 1 << iota
	Dim
	Italic
	Underline
	Reverse
	Strike
)

// Has reports whether every attribute in want is set.
func (a Attr) Has(want Attr) bool { return a&want == want }

// Style is how a cell looks. The zero Style is the terminal's own appearance.
type Style struct {
	FG, BG Color
	Attr   Attr
}

// Merge lays over on top of s: whatever over states wins, whatever it leaves at
// its default is inherited. Attributes accumulate, because an overlay that adds
// emphasis should not silently drop the emphasis underneath it.
func (s Style) Merge(over Style) Style {
	out := s
	if !over.FG.Default() {
		out.FG = over.FG
	}
	if !over.BG.Default() {
		out.BG = over.BG
	}
	out.Attr |= over.Attr
	return out
}

// span says how wide a cell is and whether it is the second column of a wide
// one. It is unexported so the head/trail pairing cannot be broken from outside
// the package: only [View.Text] creates wide cells, and it always writes both
// halves.
type span uint8

const (
	// spanSingle is the zero value, which makes a zeroed grid a grid of blanks
	// rather than a grid of orphaned continuation cells.
	spanSingle span = iota
	spanWide
	spanTrail
)

// Cell is one terminal cell.
//
// The zero Cell is a blank single-width cell in the terminal's own style, so a
// freshly allocated or cleared surface is already valid.
//
// Content is a whole grapheme cluster. A double-width cluster occupies two
// cells: the head carries the content, and the cell to its right is a trailing
// cell with no content of its own. Nothing outside this package can create half
// of such a pair.
type Cell struct {
	Content string
	Style   Style
	// Link is an OSC 8 hyperlink target. It is cell metadata rather than part of
	// Style because a hyperlink has its own open/close protocol on the wire,
	// while everything in Style is one SGR parameter list.
	Link string

	span span
}

// Width is how many columns the cell occupies: 2 for the head of a wide
// cluster, 0 for the trailing half of one, 1 otherwise.
func (c Cell) Width() int {
	switch c.span {
	case spanWide:
		return 2
	case spanTrail:
		return 0
	default:
		return 1
	}
}

// Blank reports whether the cell would print as empty space.
func (c Cell) Blank() bool { return c.Content == "" && c.span != spanTrail }
