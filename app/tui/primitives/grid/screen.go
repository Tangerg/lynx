package grid

import (
	"image"
	"io"
)

// Cursor is where the terminal's own cursor should end a frame.
type Cursor struct {
	Visible bool
	Pos     image.Point
}

// Screen is the terminal's contents, double-buffered.
//
// A frame is drawn into the back surface and flushed: the screen works out the
// smallest escape stream that turns what the terminal is showing into what was
// drawn, wraps it so the terminal applies it atomically, and swaps. Nothing above
// this type sequences escape codes, decides when to repaint, or tracks what the
// terminal already knows.
//
// A flush that would change nothing writes nothing at all — not even the frame
// markers — because an idle UI should be silent on the wire and should leave the
// cursor's blink undisturbed.
type Screen struct {
	front, back *Surface
	cursor      cursorState
	// placed is where this frame's widgets asked the cursor to go, reset by every
	// Frame and read by Flush.
	placed Cursor

	// frame and scratch are reused across flushes. scratch measures a scroll
	// candidate before committing to it.
	frame   painter
	scratch painter
	// out is the single buffer handed to the writer, so one frame is one write.
	out []byte

	// full forces the next flush to repaint every cell, because the terminal's
	// contents are no longer known: after a resize, or after something else has
	// written to the terminal.
	full bool
}

// NewScreen returns a screen of the given size whose first flush repaints
// everything.
func NewScreen(w, h int) *Screen {
	return &Screen{
		front: NewSurface(w, h),
		back:  NewSurface(w, h),
		full:  true,
	}
}

// Size returns the screen's width and height.
func (s *Screen) Size() (w, h int) { return s.back.Size() }

// Resize changes the screen's size. The next flush repaints everything: after a
// resize the terminal has reflowed its own contents, and nothing about what it is
// showing can be assumed.
func (s *Screen) Resize(w, h int) {
	if cw, ch := s.back.Size(); cw == w && ch == h {
		return
	}
	s.front.Resize(w, h)
	s.back.Resize(w, h)
	s.Invalidate()
}

// Invalidate forgets what the terminal is showing, so the next flush repaints in
// full. It is what to call after handing the terminal to another program.
func (s *Screen) Invalidate() {
	s.full = true
	s.cursor.forget()
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// Every frame draws everything it wants to be visible. Keeping content across
// frames is the diff's job, not the caller's, and a surface that carried
// yesterday's cells forward would make a missed redraw look like success.
func (s *Screen) Frame() View {
	s.back.Reset()
	s.placed = Cursor{}
	v := s.back.View()
	v.cursor = &s.placed
	return v
}

// Flush writes this frame to w, leaving the cursor wherever the frame placed it.
func (s *Screen) Flush(w io.Writer) error {
	s.frame.restart()
	s.paintCells()
	s.frame.end()
	// A frame that began is a frame that wrote cells, which is also what tells the
	// cursor it has to re-anchor.
	s.cursor.emit(&s.frame, s.placed, s.frame.begun)

	if len(s.frame.out) == 0 {
		s.swap()
		return nil
	}
	s.out = append(s.out[:0], beginSync...)
	s.out = append(s.out, s.frame.out...)
	s.out = append(s.out, endSync...)
	if _, err := w.Write(s.out); err != nil {
		// The terminal's contents are now unknown: some prefix of the frame may
		// have landed. The next flush starts over rather than diffing against a
		// front surface the terminal never fully received.
		s.Invalidate()
		return err
	}
	s.swap()
	return nil
}

// paintCells emits the cell changes for this frame, choosing between a full
// repaint, a terminal-side scroll, and a plain diff.
func (s *Screen) paintCells() {
	if s.full {
		s.full = false
		s.frame.paint(s.back, 0, s.back.h, everything)
		return
	}
	if s.paintScroll() {
		return
	}
	s.frame.paint(s.back, 0, s.back.h, changedAgainst(s.front, s.back))
}

// paintScroll takes the terminal's own scrolling shortcut when the frame is a row
// shift and the shortcut is genuinely shorter.
//
// A shift is only worth taking if it beats the diff it replaces, so each
// candidate is measured against a floor on what the diff must cost. Measuring
// against a floor rather than against the diff itself means the diff is never
// built twice.
func (s *Screen) paintScroll() bool {
	shifts := detectShifts(s.front, s.back)
	if len(shifts) == 0 {
		return false
	}
	floor := diffCostFloor(s.front, s.back)
	if floor == 0 {
		return false
	}

	best := -1
	bestLen := 0
	for i, shift := range shifts {
		s.scratch.restart()
		s.scratch.scroll(s.back, shift)
		n := len(s.scratch.out)
		if n < floor && (best < 0 || n < bestLen) {
			best, bestLen = i, n
		}
	}
	if best < 0 {
		return false
	}
	s.scratch.restart()
	s.scratch.scroll(s.back, shifts[best])
	s.frame.out = append(s.frame.out, s.scratch.out...)
	// The scratch painter established the style and cursor state that the frame
	// painter must now continue from.
	s.frame.adopt(&s.scratch)
	return true
}

// swap makes this frame the one the terminal is showing.
func (s *Screen) swap() { s.front, s.back = s.back, s.front }

// cursorState de-duplicates cursor commands across frames.
//
// The point is the blink timer: a terminal restarts it on every positioning
// command, so a UI that re-states an unchanged cursor position every frame has a
// cursor that never blinks. An idle frame must therefore say nothing at all, and
// a frame that only wrote cells must re-anchor without moving.
type cursorState struct {
	known   bool
	visible bool
	pos     image.Point
}

// forget drops the tracked state, so the next frame states everything.
func (c *cursorState) forget() { *c = cursorState{} }

// emit writes the minimal cursor commands for next.
func (c *cursorState) emit(p *painter, next Cursor, cellsChanged bool) {
	defer func() {
		c.known = true
		c.visible = next.Visible
		c.pos = next.Pos
	}()

	if !c.known {
		if next.Visible {
			p.moveTo(next.Pos.X, next.Pos.Y)
			p.out = append(p.out, showCursor...)
			return
		}
		p.out = append(p.out, hideCursor...)
		return
	}
	switch {
	case next.Visible && !c.visible:
		p.moveTo(next.Pos.X, next.Pos.Y)
		p.out = append(p.out, showCursor...)
	case !next.Visible && c.visible:
		p.out = append(p.out, hideCursor...)
	case next.Visible && (next.Pos != c.pos || cellsChanged):
		// Writing cells left the terminal's cursor wherever the last glyph
		// landed, so an unmoved cursor still has to be re-anchored.
		p.forcePos()
		p.moveTo(next.Pos.X, next.Pos.Y)
	}
}

const (
	showCursor = "\x1b[?25h"
	hideCursor = "\x1b[?25l"
)
