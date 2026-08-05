// Package present decides when a frame is drawn.
//
// A terminal UI has more reasons to redraw than it has frames worth drawing: a
// streamed token, a spinner tick, a scroll wheel and a resize can all land inside
// one refresh interval. The presenter turns that stream of reasons into a paced
// sequence of draws, and refuses to draw at all while the terminal is still
// swallowing the last frame.
//
// It is state, not machinery: it owns no goroutine, opens nothing, and writes
// nothing. The loop asks it what to do and it answers. That is what makes the
// pacing rules testable without a terminal.
package present

import "time"

// Presenter tracks whether a frame is owed, whether one is still in flight, and
// when the next one may go.
//
// Not safe for concurrent use. It belongs to the loop that draws, and the whole
// point of a single owner is that "is a frame in flight" has one answer.
type Presenter struct {
	// owed is set by every request and cleared by the draw that satisfies them
	// all. Many requests collapsing into one draw is the coalescing.
	owed bool
	// full accumulates across coalesced requests: once someone has asked for a
	// repaint from scratch, no later plain request may downgrade it.
	full bool

	// inFlight is the writer sequence this presenter is waiting to be told has
	// reached the terminal, or zero when nothing is outstanding.
	inFlight uint64

	drawnAt time.Time
	// dueAt is when a throttled request that was turned away becomes allowed.
	dueAt time.Time
}

// Request asks for a frame, now. It never draws: it records that one is owed, and
// cancels any wait a throttled request had put in the way.
func (p *Presenter) Request() {
	p.owed = true
	p.dueAt = time.Time{}
}

// RequestFull asks for a frame drawn from scratch, for when what the terminal is
// showing can no longer be trusted.
func (p *Presenter) RequestFull() {
	// Through Request, so a repaint is never left waiting behind a throttled stream: the
	// terminal's contents are no longer known, and pacing a correction is pointless.
	p.Request()
	p.full = true
}

// RequestBy asks for a frame no sooner than minInterval after the last one, and reports
// whether it may be drawn straight away.
//
// It is for sources that fire faster than a terminal can usefully be redrawn — a token
// stream, a scroll wheel held down. A request too soon still owes a frame: the interval
// decides when it is drawn, not whether. It arms [Presenter.DueAt] so a loop that parks
// knows when to wake, and [Presenter.Present] holds the frame until then.
func (p *Presenter) RequestBy(now time.Time, minInterval time.Duration) bool {
	p.owed = true
	if now.Sub(p.drawnAt) < minInterval {
		if p.dueAt.IsZero() {
			p.dueAt = p.drawnAt.Add(minInterval)
		}
		return false
	}
	p.dueAt = time.Time{}
	return true
}

// Present draws the owed frame, if one is owed and the terminal is not still busy
// with the last, and reports whether it drew.
//
// draw is given whether this frame must be a repaint from scratch, and returns the
// writer sequence its bytes were queued under — zero when it queued nothing. A
// frame that reached the writer becomes the one being waited for, so the next
// request coalesces instead of piling a second frame behind the first.
func (p *Presenter) Present(now time.Time, draw func(full bool) uint64) bool {
	if p.inFlight != 0 || !p.owed {
		return false
	}
	if !p.dueAt.IsZero() && now.Before(p.dueAt) {
		return false
	}
	full := p.full
	p.owed, p.full = false, false
	if seq := draw(full); seq != 0 {
		p.inFlight = seq
	}
	p.drawnAt = now
	p.dueAt = time.Time{}
	return true
}

// Wrote reports that the writer has finished with everything up to seq. The frame
// being waited for is released once seq reaches it.
func (p *Presenter) Wrote(seq uint64) {
	if p.inFlight != 0 && seq >= p.inFlight {
		p.inFlight = 0
	}
}

// DueAt is when a turned-away throttled request becomes allowed, if one is pending.
//
// A loop that parks until something happens has to know to wake then, or the last
// update of a burst is never drawn.
func (p *Presenter) DueAt() (time.Time, bool) {
	return p.dueAt, !p.dueAt.IsZero()
}
