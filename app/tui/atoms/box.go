package atoms

import (
	"image"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/text"
)

// Border is the set of characters a box is drawn with. The zero Border draws no
// lines, which is what a box that only pads its content wants.
type Border struct {
	Top, Bottom, Left, Right string
	TopLeft, TopRight        string
	BottomLeft, BottomRight  string
}

// Rounded and Square are the two line styles worth having. Rounded reads as a
// panel and Square as a table; anything heavier competes with the content.
var (
	Rounded = Border{
		Top: "─", Bottom: "─", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
	}
	Square = Border{
		Top: "─", Bottom: "─", Left: "│", Right: "│",
		TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘",
	}
)

// drawn reports whether the border draws anything.
func (b Border) drawn() bool { return b != Border{} }

// Inset is space held clear on each side.
type Inset struct{ Top, Right, Bottom, Left int }

// Uniform is the same inset on every side.
func Uniform(n int) Inset { return Inset{Top: n, Right: n, Bottom: n, Left: n} }

// Symmetric is one inset above and below, another to the left and right — the
// common case, because a terminal cell is about twice as tall as it is wide and
// even padding does not look even.
func Symmetric(vertical, horizontal int) Inset {
	return Inset{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

// Box frames and pads a region.
//
// It is not a container: it does not own a child. A caller draws the box and then
// draws into the view the box hands back, which keeps the box out of the question
// of what goes inside it and lets the same box frame a widget, a string, or nothing.
type Box struct {
	Border Border
	// Padding is held clear inside the border.
	Padding Inset
	// Style paints the border and the padding.
	Style grid.Style
	// Fill paints the interior before anything is drawn into it. Its zero value
	// leaves the interior as the terminal had it.
	Fill grid.Style
	// Title sits in the top border, indented one column so the corner reads as a
	// corner.
	Title      string
	TitleStyle grid.Style
	TitleAlign Align
	// Footer sits in the bottom border, on the same terms as the title.
	Footer      string
	FooterStyle grid.Style
	FooterAlign Align
}

// Overhead is how many columns and rows the frame and padding take, which is what
// a caller subtracts to know what is left for the content.
func (b Box) Overhead() (w, h int) {
	edge := 0
	if b.Border.drawn() {
		edge = 2
	}
	return edge + b.Padding.Left + b.Padding.Right, edge + b.Padding.Top + b.Padding.Bottom
}

// Inner is the region left for content, in v's coordinates.
func (b Box) Inner(v grid.View) grid.View {
	w, h := v.Size()
	edge := 0
	if b.Border.drawn() {
		edge = 1
	}
	x := edge + b.Padding.Left
	y := edge + b.Padding.Top
	ow, oh := b.Overhead()
	return v.Sub(grid.Rect(x, y, max(w-ow, 0), max(h-oh, 0)))
}

// Draw paints the frame and returns the region left for content, so the common use
// reads as one step:
//
//	inner := box.Draw(v)
//	content.Draw(inner)
func (b Box) Draw(v grid.View) grid.View {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return v.Sub(image.Rectangle{})
	}
	if b.Fill != (grid.Style{}) {
		v.Fill(grid.Rect(0, 0, w, h), b.Fill)
	}
	if b.Border.drawn() {
		b.drawBorder(v, w, h)
	}
	return b.Inner(v)
}

func (b Box) drawBorder(v grid.View, w, h int) {
	// A box one column or one row deep has no room for two opposing edges. Drawing
	// what fits and no more keeps a collapsing layout from looking corrupted.
	if h >= 1 {
		b.drawEdge(v, 0, w, b.Border.TopLeft, b.Border.Top, b.Border.TopRight)
	}
	if h >= 2 {
		b.drawEdge(v, h-1, w, b.Border.BottomLeft, b.Border.Bottom, b.Border.BottomRight)
	}
	for y := 1; y < h-1; y++ {
		v.Text(0, y, b.Border.Left, b.Style)
		if w >= 2 {
			v.Text(w-1, y, b.Border.Right, b.Style)
		}
	}
	if h >= 1 {
		b.label(v, 0, w, b.Title, b.TitleStyle, b.TitleAlign)
	}
	if h >= 2 {
		b.label(v, h-1, w, b.Footer, b.FooterStyle, b.FooterAlign)
	}
}

func (b Box) drawEdge(v grid.View, y, w int, left, mid, right string) {
	v.Text(0, y, left, b.Style)
	for x := 1; x < w-1; x++ {
		v.Text(x, y, mid, b.Style)
	}
	if w >= 2 {
		v.Text(w-1, y, right, b.Style)
	}
}

// label writes a title or footer into a border row, keeping a column of border on
// each side of it so the line still reads as a frame.
func (b Box) label(v grid.View, y, w int, label string, style grid.Style, align Align) {
	if label == "" || w <= 4 {
		return
	}
	room := w - 4
	label = text.Truncate(label, room, "…")
	width := text.Width(label)
	x := 2 + align.offset(room, width)
	v.Text(x, y, label, style)
}
