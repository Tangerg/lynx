package grid

import (
	"image"
	"io"
	"strconv"
)

// eraseLine clears from the cursor to the end of its row. It is what makes a row
// that got shorter actually get shorter, rather than keeping the tail of what it
// used to say.
const eraseLine = "\x1b[K"

// Inline draws an interface as a block in the terminal's own screen, with output
// that is finished printed above it.
//
// It is the other way to put frames on a terminal, and the one that makes a
// program part of a session rather than a mode of it: what the interface has
// already said stays in the terminal's own scrollback, where the user can scroll
// back to it, select it, and see it still there after the program exits. A
// [Screen] takes a screen of its own and gives back a blank terminal; this keeps
// the transcript.
//
// # Why nothing here is addressed absolutely
//
// The block's position on the terminal is decided by whatever is above it, which
// this type does not own and cannot ask about. So every frame is written relative
// to where the last one left the cursor: back to the top of the block, down through
// its rows, and back to wherever the cursor belongs. Printing works the same way —
// the rows are written where the block's first row was, and the block is drawn
// below them, which is what pushes finished output up and into the scrollback.
//
// The block is as tall as what was drawn: the rows up to the last one with anything
// on it, and never fewer than enough to hold the cursor. Nothing has to declare a
// height, and an interface that draws two rows occupies two rows.
//
// # What a resize costs
//
// A resize is the one thing this cannot get exactly right. The terminal may reflow
// what is above the block, and there is no way to ask where the block ended up, so
// the next frame repaints in full from where the cursor was left. That is exact
// when the terminal did not reflow and approximate when it did, which is the same
// bargain every inline interface makes.
type Inline struct {
	front, back *Surface
	// scratch is where printed rows are drawn before being encoded. It belongs to
	// the block so it is always the block's width, which is the one width a printed
	// row may be without the terminal wrapping it and moving the block.
	scratch *Surface

	// placed is where this frame's widgets asked the cursor to go, reset by every
	// [Inline.Frame] and read by [Inline.Flush].
	placed Cursor

	// pending is output that has been printed but not yet written, already encoded.
	// A printed row never changes again, so keeping its cells would buy nothing.
	pending []string

	// rows is how tall the block was after the last flush, and at is where that
	// flush left the terminal's cursor, in the block's own coordinates. Together
	// they are the anchor every frame is written relative to.
	rows int
	at   image.Point

	// known and shown are what the terminal has been told about its cursor, so an
	// idle frame says nothing and leaves the blink alone.
	known bool
	shown bool

	// full forces the next flush to rewrite every row of the block.
	full bool

	// buf is one frame's payload and out the same wrapped for atomic application.
	buf, out []byte
}

// NewInline returns an inline block that may grow to h rows of w columns, whose
// first flush draws everything.
//
// The height is a ceiling rather than a size: it is what the terminal can spare,
// and the block takes as much of it as the interface draws into.
func NewInline(w, h int) *Inline {
	return &Inline{
		front:   NewSurface(w, h),
		back:    NewSurface(w, h),
		scratch: NewSurface(w, 0),
		full:    true,
	}
}

// Size returns the block's width and the height it may grow to.
func (i *Inline) Size() (w, h int) { return i.back.Size() }

// Resize changes the width and the height the block may grow to.
func (i *Inline) Resize(w, h int) {
	if cw, ch := i.back.Size(); cw == w && ch == h {
		return
	}
	i.front.Resize(w, h)
	i.back.Resize(w, h)
	i.Invalidate()
}

// Invalidate forgets what the terminal is showing, so the next flush rewrites the
// whole block.
func (i *Inline) Invalidate() {
	i.full = true
	i.known = false
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// The view is as tall as the block may grow to, not as tall as the block is: how
// tall it is is decided by what this frame draws into it.
func (i *Inline) Frame() View {
	i.back.Reset()
	i.placed = Cursor{}
	v := i.back.View()
	v.cursor = &i.placed
	return v
}

// Print draws rows that become part of the terminal's own output, above the
// interface, and stay there.
//
// The rows are drawn now, into a surface as wide as the block, and kept as the
// text they came to. They reach the terminal with the next flush, before the block,
// which is what puts them above it.
//
// It takes a count rather than working one out because output can be taller than
// the terminal — a long answer printed into the scrollback is the ordinary case —
// and a caller that has laid its content out already knows how tall it is. Every
// row asked for is printed, so a blank row is a blank row and not slack.
func (i *Inline) Print(rows int, draw func(View)) {
	if rows <= 0 {
		return
	}
	w, _ := i.back.Size()
	i.scratch.Resize(w, rows)
	if draw != nil {
		draw(i.scratch.View())
	}
	for y := range rows {
		i.pending = append(i.pending, EncodeRow(i.scratch.Row(y)))
	}
}

// Flush writes this frame to w, leaving the cursor wherever the frame placed it.
//
// A flush that would change nothing writes nothing at all, for the same reason a
// [Screen] does: an idle interface should be silent on the wire and should leave
// the cursor's blink undisturbed.
func (i *Inline) Flush(w io.Writer) error {
	used := i.used()
	i.buf = i.buf[:0]
	i.compose(used)
	if len(i.buf) == 0 {
		i.settle(used)
		return nil
	}
	i.out = append(i.out[:0], beginSync...)
	i.out = append(i.out, i.buf...)
	i.out = append(i.out, endSync...)
	if _, err := w.Write(i.out); err != nil {
		// Some prefix of the frame may have landed, so what the terminal is showing
		// is no longer known. The printed rows are kept rather than dropped: output
		// the caller asked for is worth writing twice and not worth losing.
		i.Invalidate()
		return err
	}
	i.pending = i.pending[:0]
	i.settle(used)
	return nil
}

// Finish leaves the block on screen with the cursor below it, so whatever writes
// next — the shell's prompt, or this program's own output — starts on a line of its
// own instead of on top of the interface.
//
// It is the counterpart of giving back the alternate screen, and the reason an
// inline program has to draw one last frame before it exits: the last thing it
// showed is the thing that stays.
func (i *Inline) Finish(w io.Writer) error {
	i.buf = i.buf[:0]
	if i.rows > 0 {
		if down := i.rows - 1 - i.at.Y; down > 0 {
			i.csi(down, 'B')
		}
		i.buf = append(i.buf, "\r\n"...)
	}
	i.buf = append(i.buf, sgrReset...)
	i.buf = append(i.buf, showCursor...)
	i.rows, i.at = 0, image.Point{}
	i.Invalidate()
	_, err := w.Write(i.buf)
	return err
}

// used is how many rows the block needs: the rows up to the last one with anything
// on it, and enough to hold the cursor if a widget placed one.
func (i *Inline) used() int {
	_, h := i.back.Size()
	last := -1
	for y := h - 1; y >= 0; y-- {
		if len(trimBlankTail(i.back.Row(y))) > 0 {
			last = y
			break
		}
	}
	if i.placed.Visible {
		last = max(last, i.placed.Pos.Y)
	}
	return last + 1
}

// compose builds this frame's payload, or leaves it empty when the terminal is
// already showing this frame.
func (i *Inline) compose(used int) {
	// Printing rewrites the rows the block's first rows were on and moves the block
	// down past them, so nothing the block is showing survives it.
	printed := len(i.pending)
	full := i.full || printed > 0

	changed := func(y int) bool { return full || !rowEqual(i.front, y, i.back, y) }

	// The rows the block is giving up. They are erased where they are rather than
	// deleted, which leaves the block shorter with blank rows below it and moves
	// nothing that is above it.
	extra := max(i.rows-printed-used, 0)

	work := full || extra > 0
	for y := 0; y < used && !work; y++ {
		work = changed(y)
	}
	if !work && !i.cursorPending() {
		return
	}

	// The terminal's style at the start of a frame is not knowable — another program
	// may have written to it — so a frame that writes anything makes it knowable.
	i.buf = append(i.buf, sgrReset...)

	// Back to the top of the block, which is the only position this type can name.
	i.buf = append(i.buf, '\r')
	if i.at.Y > 0 {
		i.csi(i.at.Y, 'A')
	}

	for _, line := range i.pending {
		i.buf = append(i.buf, line...)
		i.buf = append(i.buf, eraseLine...)
		i.buf = append(i.buf, "\r\n"...)
	}

	// moved tracks whether the row just visited left the cursor away from column
	// zero, which is the only thing a carriage return is for.
	moved := false
	total := used + extra
	for y := range total {
		if y > 0 {
			// The carriage return cancels the terminal's pending wrap before the
			// newline: a row that filled the last column would otherwise advance
			// twice and take the block's anchor with it.
			i.buf = append(i.buf, "\r\n"...)
			moved = false
		}
		switch {
		case y >= used:
			// Erasing leaves the cursor where it is, which is already column zero.
			i.buf = append(i.buf, eraseLine...)
		case changed(y):
			row := EncodeRow(i.back.Row(y))
			i.buf = append(i.buf, row...)
			i.buf = append(i.buf, eraseLine...)
			moved = len(row) > 0
		}
	}
	if moved {
		i.buf = append(i.buf, '\r')
	}

	at := image.Pt(0, max(used-1, 0))
	if up := max(total-1, 0) - at.Y; up > 0 {
		i.csi(up, 'A')
	}
	i.placeCursor(at)
}

// cursorPending reports whether the terminal has to be told something about its
// cursor that it does not already know.
func (i *Inline) cursorPending() bool {
	if !i.known || i.placed.Visible != i.shown {
		return true
	}
	return i.placed.Visible && i.placed.Pos != i.at
}

// placeCursor moves the cursor from at, where writing the rows left it, to where
// this frame's widgets asked for it.
func (i *Inline) placeCursor(at image.Point) {
	i.at = at
	defer func() { i.known, i.shown = true, i.placed.Visible }()

	if !i.placed.Visible {
		if !i.known || i.shown {
			i.buf = append(i.buf, hideCursor...)
		}
		return
	}
	// The cursor is never below the block: a frame that placed one counts its row as
	// drawn, which is what makes the block tall enough to hold it.
	if up := at.Y - i.placed.Pos.Y; up > 0 {
		i.csi(up, 'A')
	}
	if i.placed.Pos.X > 0 {
		i.csi(i.placed.Pos.X, 'C')
	}
	i.at = i.placed.Pos
	if !i.known || !i.shown {
		i.buf = append(i.buf, showCursor...)
	}
}

// settle makes this frame the one the terminal is showing.
func (i *Inline) settle(used int) {
	i.front, i.back = i.back, i.front
	i.rows = used
	i.full = false
}

func (i *Inline) csi(n int, final byte) {
	i.buf = append(i.buf, '\x1b', '[')
	i.buf = strconv.AppendInt(i.buf, int64(n), 10)
	i.buf = append(i.buf, final)
}
