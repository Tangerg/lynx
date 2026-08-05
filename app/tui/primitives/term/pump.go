package term

import (
	"time"

	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// escGrace is how long a lone escape byte is held before it is taken to be the
// Escape key rather than the start of a sequence whose rest has not arrived.
//
// Long enough that a sequence sent in one burst always arrives whole, short enough
// that pressing Escape does not feel late. Escape sequences travel together; the
// gap only opens when a human is typing.
const escGrace = 30 * time.Millisecond

// pump turns raw terminal bytes into events.
//
// It is separated from the terminal it normally reads because the interesting part
// has nothing to do with a terminal: deciding when a buffered escape has waited
// long enough is a matter of timing, and timing is what a real terminal makes
// impossible to test.
type pump struct {
	// raw carries byte chunks exactly as they were read.
	raw <-chan []byte
	// readErr carries the end of the input, by error or by end of file.
	readErr <-chan error
	// resized fires when the terminal's size may have changed.
	resized <-chan struct{}
	// stop asks the pump to return.
	stop <-chan struct{}

	// out receives the decoded events, and is closed when the pump returns so a
	// consumer ranging over it learns that input is over.
	out chan input.Event
	// size reports the terminal's current size.
	size func() (w, h int, err error)
	// grace overrides escGrace, for tests.
	grace time.Duration
}

// run decodes until the input ends or the pump is asked to stop, then closes out.
func (p *pump) run() {
	defer close(p.out)

	grace := p.grace
	if grace <= 0 {
		grace = escGrace
	}
	var parser input.Parser

	// A stopped timer with a drained channel, so arming and disarming it is a
	// matter of Reset and Stop and never of a stale tick arriving late.
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	armed := false
	disarm := func() {
		if armed && !timer.Stop() {
			<-timer.C
		}
		armed = false
	}

	for {
		select {
		case chunk := <-p.raw:
			disarm()
			if !p.deliver(parser.Feed(chunk)) {
				return
			}
			if parser.Pending() {
				// Something is waiting on bytes that may never come. Only time can
				// settle it.
				timer.Reset(grace)
				armed = true
			}
		case <-timer.C:
			armed = false
			if !p.deliver(parser.Flush()) {
				return
			}
		case <-p.resized:
			if w, h, err := p.size(); err == nil {
				if !p.deliver([]input.Event{input.Resize{Width: w, Height: h}}) {
					return
				}
			}
		case <-p.readErr:
			// The input is over. Bytes that arrived before it ended are still the
			// user's — they and the end arrive on separate channels, and a select
			// cannot be told to prefer one, so whichever this pass happened to see
			// first says nothing about which happened first. Everything already
			// waiting is taken before anything is given up.
			if !p.drainRaw(&parser) {
				return
			}
			p.deliver(parser.Flush())
			return
		case <-p.stop:
			return
		}
	}
}

// drainRaw feeds everything already waiting on raw, reporting false when the pump was
// asked to stop part-way through.
func (p *pump) drainRaw(parser *input.Parser) bool {
	for {
		select {
		case chunk := <-p.raw:
			if !p.deliver(parser.Feed(chunk)) {
				return false
			}
		default:
			return true
		}
	}
}

// deliver sends events on, reporting false when the pump was asked to stop
// part-way through. Stopping mid-batch loses the rest, which is correct: nothing
// downstream is listening any more.
func (p *pump) deliver(events []input.Event) bool {
	for _, ev := range events {
		select {
		case p.out <- ev:
		case <-p.stop:
			return false
		}
	}
	return true
}
