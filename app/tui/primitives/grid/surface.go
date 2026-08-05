package grid

import (
	"image"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// Surface is a rectangle of cells in row-major order. It is storage and
// geometry; drawing happens through the [View] it hands out, so no caller has to
// carry a clip rectangle alongside the buffer it is clipping.
type Surface struct {
	w, h  int
	cells []Cell
}

// NewSurface returns a blank surface of the given size.
func NewSurface(w, h int) *Surface {
	s := &Surface{}
	s.Resize(w, h)
	return s
}

// Resize changes the surface's size and blanks it. Content is not preserved:
// every resize is followed by a full redraw, so carrying stale cells across one
// would only make the first frame after it wrong in a subtler way.
func (s *Surface) Resize(w, h int) {
	w, h = max(w, 0), max(h, 0)
	s.w, s.h = w, h
	if n := w * h; cap(s.cells) >= n {
		s.cells = s.cells[:n]
	} else {
		s.cells = make([]Cell, n)
	}
	s.Reset()
}

// Reset blanks every cell.
func (s *Surface) Reset() { clear(s.cells) }

// Size returns the surface's width and height.
func (s *Surface) Size() (w, h int) { return s.w, s.h }

// Bounds is the surface's own rectangle, with its origin at zero.
func (s *Surface) Bounds() image.Rectangle { return Rect(0, 0, s.w, s.h) }

// View returns a drawing view over the whole surface.
func (s *Surface) View() View {
	if s == nil {
		return View{}
	}
	return View{surface: s, size: image.Pt(s.w, s.h), clip: s.Bounds()}
}

// CellAt returns the cell at (x, y), or nil when the coordinates are outside the
// surface. The cell is addressable so a reader can inspect what was drawn;
// writing through it bypasses the wide-pair invariant and is a bug.
func (s *Surface) CellAt(x, y int) *Cell {
	if s == nil || x < 0 || x >= s.w || y < 0 || y >= s.h {
		return nil
	}
	return &s.cells[y*s.w+x]
}

// Row returns the cells of one row, or nil when y is outside the surface.
func (s *Surface) Row(y int) []Cell {
	if s == nil || y < 0 || y >= s.h {
		return nil
	}
	return s.cells[y*s.w : (y+1)*s.w]
}

// CopyRows copies n whole rows out of src, starting at srcTop, into s starting at
// dstTop. Rows that fall outside either surface are skipped, which is what lets a
// caller render an over-tall item into a scratch surface and lift the visible
// slice of it into place.
func (s *Surface) CopyRows(src *Surface, srcTop, dstTop, n int) {
	if s == nil || src == nil || src.w != s.w {
		return
	}
	for i := range n {
		sy, dy := srcTop+i, dstTop+i
		if sy < 0 || sy >= src.h || dy < 0 || dy >= s.h {
			continue
		}
		copy(s.Row(dy), src.Row(sy))
	}
}

// repairPair keeps the wide-pair invariant across an overwrite of (x, y): when
// the cell about to be replaced is half of a wide cluster, its partner is blanked
// so no orphaned head or trailing cell survives.
func (s *Surface) repairPair(x, y int) {
	i := y*s.w + x
	c := &s.cells[i]
	if c.span == spanTrail && x > 0 {
		if head := &s.cells[i-1]; head.span == spanWide {
			*head = Cell{Style: head.Style}
		}
	}
	if c.span == spanWide && x+1 < s.w {
		if trail := &s.cells[i+1]; trail.span == spanTrail {
			*trail = Cell{Style: trail.Style}
		}
	}
}

// View is a clipped window onto a [Surface], addressed in its own coordinates.
//
// A view is how everything above this package draws. Handing a widget a view
// sized to its box means it cannot draw outside that box: coordinates are local,
// and anything landing beyond the clip is discarded rather than trusted.
//
// The zero View draws nowhere and reports a size of zero, which is the right
// answer for a widget laid out into no space at all.
type View struct {
	surface *Surface
	// origin is where this view's (0, 0) sits on the surface.
	origin image.Point
	// size is the box the view was laid out into, which is not the same as what
	// it may draw on: a widget half scrolled off the screen still lays out for
	// its whole size and simply loses the part outside the clip.
	size image.Point
	// clip is the region of the surface this view may touch, in surface
	// coordinates, never wider than the surface itself.
	clip image.Rectangle
	// cursor is where the frame's terminal cursor is recorded, shared by every
	// view of the frame. It is nil for a surface that is not a frame, where
	// placing a cursor is meaningless rather than an error.
	cursor *Cursor
}

// Size returns the box the view was laid out into.
func (v View) Size() (w, h int) { return v.size.X, v.size.Y }

// Bounds is the view's own coordinate space, origin at zero.
func (v View) Bounds() image.Rectangle { return image.Rectangle{Max: v.size} }

// Visible is the part of the view that will actually reach the screen, in the
// view's own coordinates. It is empty for a view with nowhere to draw.
func (v View) Visible() image.Rectangle {
	if v.surface == nil {
		return image.Rectangle{}
	}
	return v.clip.Sub(v.origin)
}

// Empty reports whether the view has nowhere to draw.
func (v View) Empty() bool { return v.surface == nil || v.clip.Empty() }

// Sub returns a view onto r, expressed in this view's coordinates. Clipping only
// ever narrows: a widget cannot hand a child room it does not have itself.
func (v View) Sub(r image.Rectangle) View {
	if v.surface == nil {
		return View{}
	}
	return View{
		surface: v.surface,
		origin:  v.origin.Add(r.Min),
		size:    r.Size(),
		clip:    v.clip.Intersect(r.Add(v.origin)),
		cursor:  v.cursor,
	}
}

// PlaceCursor asks for the terminal's cursor to sit at local (x, y).
//
// It is how the one widget that owns the cursor says so, without anyone in between
// having to carry the answer: the view already knows where it sits on the screen,
// so the widget speaks in its own coordinates and the translation is nobody's job.
//
// A position outside what the view may draw on is ignored, for the same reason a
// glyph there would be: a widget scrolled off the screen does not get to move the
// cursor. A frame in which nobody places the cursor is a frame with no cursor,
// which is the right answer when nothing is being typed into.
func (v View) PlaceCursor(x, y int) {
	if v.cursor == nil {
		return
	}
	p := image.Pt(x, y).Add(v.origin)
	if !p.In(v.clip) {
		return
	}
	*v.cursor = Cursor{Visible: true, Pos: p}
}

// CellAt returns the cell at local (x, y), or nil when it is outside the clip.
func (v View) CellAt(x, y int) *Cell {
	p := image.Pt(x, y).Add(v.origin)
	if v.surface == nil || !p.In(v.clip) {
		return nil
	}
	return v.surface.CellAt(p.X, p.Y)
}

// Fill blanks every cell in r, in this view's coordinates, and gives it style.
func (v View) Fill(r image.Rectangle, style Style) {
	if v.surface == nil {
		return
	}
	area := v.clip.Intersect(r.Add(v.origin))
	if area.Empty() {
		return
	}
	s := v.surface
	for y := area.Min.Y; y < area.Max.Y; y++ {
		// A fill edge can land in the middle of a wide cluster; repairing both
		// edges first keeps the half left outside the fill from being orphaned.
		s.repairPair(area.Min.X, y)
		s.repairPair(area.Max.X-1, y)
		row := s.Row(y)
		for x := area.Min.X; x < area.Max.X; x++ {
			row[x] = Cell{Style: style}
		}
	}
}

// Text writes s at local (x, y) and returns how many columns it advanced,
// including any it advanced outside the clip.
//
// Text is grapheme-aware. A double-width cluster takes two columns and is never
// split: one that would straddle the right edge is dropped and its first column
// blanked, because half a glyph is worse than a gap. A zero-width cluster — a
// combining mark arriving on its own — joins the cell to its left instead of
// consuming a column of its own.
func (v View) Text(x, y int, s string, style Style) int {
	if v.surface == nil {
		return 0
	}
	p := image.Pt(x, y).Add(v.origin)
	if p.Y < v.clip.Min.Y || p.Y >= v.clip.Max.Y {
		return graphemeWidth(s)
	}

	surf := v.surface
	cx := p.X
	state := -1
	var cluster string
	for len(s) > 0 {
		cluster, s, _, state = uniseg.StepString(s, state)
		w := ClusterWidth(cluster)
		if control(cluster) {
			continue
		}
		if w == 0 {
			v.combine(cx, p.Y, cluster)
			continue
		}
		if cx >= v.clip.Max.X {
			break
		}
		switch {
		case cx+w <= v.clip.Min.X:
			// Entirely left of the clip.
		case cx < v.clip.Min.X:
			// A wide cluster straddling the left edge: blank the column that is
			// inside rather than print a glyph the terminal would place wrong.
			surf.repairPair(v.clip.Min.X, p.Y)
			*surf.CellAt(v.clip.Min.X, p.Y) = Cell{Style: style}
		case w == 2 && cx+2 > v.clip.Max.X:
			surf.repairPair(cx, p.Y)
			*surf.CellAt(cx, p.Y) = Cell{Style: style}
			// Nothing wider than one column can follow on this row.
			return cx + 1 - p.X
		case w == 2:
			surf.repairPair(cx, p.Y)
			surf.repairPair(cx+1, p.Y)
			*surf.CellAt(cx, p.Y) = Cell{Content: cluster, Style: style, span: spanWide}
			*surf.CellAt(cx+1, p.Y) = Cell{Style: style, span: spanTrail}
		default:
			surf.repairPair(cx, p.Y)
			*surf.CellAt(cx, p.Y) = Cell{Content: cluster, Style: style}
		}
		cx += w
	}
	return cx - p.X
}

// combine appends a zero-width cluster to the cell that owns the column to the
// left, stepping over a trailing cell to reach its head.
func (v View) combine(cx, y int, cluster string) {
	prev := v.surface.CellAt(cx-1, y)
	if prev != nil && prev.span == spanTrail {
		prev = v.surface.CellAt(cx-2, y)
	}
	if prev == nil || prev.span == spanTrail {
		return
	}
	prev.Content += cluster
}

// Link stamps target onto w columns starting at local (x, y), turning text that
// has already been written into a hyperlink. It is separate from [View.Text]
// because a link usually spans a run that was drawn in several pieces.
func (v View) Link(x, y, w int, target string) {
	for i := range w {
		if c := v.CellAt(x+i, y); c != nil {
			c.Link = target
		}
	}
}

// graphemeWidth is how many columns s would occupy.
func graphemeWidth(s string) int {
	total := 0
	state := -1
	var cluster string
	for len(s) > 0 {
		cluster, s, _, state = uniseg.StepString(s, state)
		total += ClusterWidth(cluster)
	}
	return total
}

// columns is the width table every measurement in the TUI goes through.
//
// It is built explicitly instead of using the package-level default, whose
// East Asian width setting is decided by the locale environment variables of
// whatever machine the program happens to run on. That would make a character
// like "…" one column wide here and two there — the same layout code producing
// different frames, and golden output that passes on one developer's machine and
// fails on another's. Ambiguous-width characters are narrow, which is what a
// terminal not told otherwise does with them.
var columns = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

// ClusterWidth is how many columns one grapheme cluster occupies, clamped to what
// a cell can hold. A control character measures zero.
//
// Everything that lays text out shares this function. Measuring text one way and
// drawing it another is the cause of every misaligned terminal UI, so there is one
// answer and one place it comes from.
func ClusterWidth(cluster string) int {
	if control(cluster) {
		return 0
	}
	return min(max(columns.StringWidth(cluster), 0), 2)
}

// control reports whether a cluster begins with a control character.
//
// Such a cluster is dropped rather than stored. A control byte living in a cell
// would be written to the terminal verbatim on the next repaint — a tab or
// carriage return would move the cursor out from under the renderer, and an
// escape would begin a sequence the terminal obeys. Cells are filled from tool
// output and model output, so this is a trust boundary and not a tidiness rule.
// Anything above this package that wants a tab to occupy columns expands it
// first, where the column it starts at is known.
func control(cluster string) bool {
	if cluster == "" {
		return false
	}
	b := cluster[0]
	return b < 0x20 || b == 0x7f
}
