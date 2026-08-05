package term

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

func TestEveryModeTurnedOnIsTurnedBackOff(t *testing.T) {
	// A mode left on outlives the process: mouse reporting still on means the shell
	// prints escape sequences when the user moves the mouse, and the alternate
	// screen still on means whatever was on screen before is gone.
	all := modes{altScreen: true, mouse: true, focus: true, keyboard: true}
	enter, leave := all.enter(), all.leave()

	for _, pair := range all.sequence() {
		if !strings.Contains(enter, pair.on) {
			t.Errorf("entering does not turn on %q", pair.on)
		}
		if !strings.Contains(leave, pair.off) {
			t.Errorf("leaving does not turn off %q", pair.on)
		}
	}
}

func TestModesAreUndoneInTheOppositeOrder(t *testing.T) {
	// The alternate screen is entered first and has to be left last, or the modes
	// underneath are put back onto a screen that is about to be discarded.
	all := modes{altScreen: true, mouse: true, focus: true, keyboard: true}
	enter, leave := all.enter(), all.leave()

	seq := all.sequence()
	for i := range seq {
		for j := i + 1; j < len(seq); j++ {
			if strings.Index(enter, seq[i].on) > strings.Index(enter, seq[j].on) {
				t.Fatalf("%q is turned on after %q", seq[i].on, seq[j].on)
			}
			if strings.Index(leave, seq[i].off) < strings.Index(leave, seq[j].off) {
				t.Fatalf("%q is turned off before %q", seq[i].off, seq[j].off)
			}
		}
	}
}

func TestAModeNotAskedForIsNeverTouched(t *testing.T) {
	none := modes{}
	enter, leave := none.enter(), none.leave()
	for _, unwanted := range []string{altScreenOn, mouseOn, focusOn, keyboardOn} {
		if strings.Contains(enter, unwanted) {
			t.Errorf("entering turned on %q without being asked", unwanted)
		}
	}
	for _, unwanted := range []string{altScreenOff, mouseOff, focusOff, keyboardOff} {
		if strings.Contains(leave, unwanted) {
			t.Errorf("leaving turned off %q that was never on", unwanted)
		}
	}
	// Bracketed paste is not optional: without it a pasted block arrives as
	// keystrokes, and pasted code runs.
	if !strings.Contains(enter, pasteOn) || !strings.Contains(leave, pasteOff) {
		t.Error("bracketed paste is not handled unconditionally")
	}
}

func TestLeavingAlwaysShowsTheCursor(t *testing.T) {
	// A frame may have hidden it, and a hidden cursor in the shell afterwards looks
	// like a hung terminal.
	if !strings.HasSuffix(modes{}.leave(), cursorShow) {
		t.Error("leaving does not end by showing the cursor")
	}
}

func TestOpenWithoutATerminal(t *testing.T) {
	// Under a test runner standard input is not a terminal, which is exactly the
	// case a caller has to handle rather than force: a program whose output is piped
	// wants text, not frames.
	if _, err := Open(Options{}); !errors.Is(err, ErrNotTerminal) {
		t.Fatalf("Open error = %v, want ErrNotTerminal", err)
	}
}

// driver runs a pump over channels a test controls.
type driver struct {
	raw     chan []byte
	readErr chan error
	resized chan struct{}
	stop    chan struct{}
	events  chan input.Event
	size    func() (int, int, error)
	done    chan struct{}
}

func newDriver(grace time.Duration) *driver {
	d := &driver{
		raw:     make(chan []byte, 4),
		readErr: make(chan error, 1),
		resized: make(chan struct{}, 1),
		stop:    make(chan struct{}),
		events:  make(chan input.Event, 64),
		size:    func() (int, int, error) { return 80, 24, nil },
		done:    make(chan struct{}),
	}
	p := &pump{
		raw: d.raw, readErr: d.readErr, resized: d.resized, stop: d.stop,
		out: d.events, size: func() (int, int, error) { return d.size() }, grace: grace,
	}
	go func() {
		defer close(d.done)
		p.run()
	}()
	return d
}

// next waits for one event.
func (d *driver) next(t *testing.T) input.Event {
	t.Helper()
	select {
	case ev, ok := <-d.events:
		if !ok {
			t.Fatal("the event channel closed while an event was expected")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
		return nil
	}
}

// silent asserts that nothing arrives within d.
func (d *driver) silent(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case ev := <-d.events:
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(within):
	}
}

func TestPumpDecodesBytesIntoEvents(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	defer close(d.stop)

	d.raw <- []byte("a\x1b[B")
	if got := d.next(t).(input.Key); !got.IsRune('a', 0) {
		t.Fatalf("first = %+v", got)
	}
	if got := d.next(t).(input.Key).Code; got != input.Down {
		t.Fatalf("second = %v", got)
	}
}

func TestPumpHoldsALoneEscapeThenDeliversIt(t *testing.T) {
	const grace = 40 * time.Millisecond
	d := newDriver(grace)
	defer close(d.stop)

	d.raw <- []byte("\x1b")
	// Held: it could still become a sequence, and guessing early would turn every
	// arrow key into an Escape followed by a letter.
	d.silent(t, grace/2)
	if got := d.next(t).(input.Key).Code; got != input.Esc {
		t.Fatalf("event = %v, want the Escape key once the wait was over", got)
	}
}

func TestPumpDisarmsTheWaitWhenTheSequenceArrives(t *testing.T) {
	d := newDriver(40 * time.Millisecond)
	defer close(d.stop)

	// The rest of the sequence arrives in a second read, which is normal.
	d.raw <- []byte("\x1b")
	d.raw <- []byte("[A")
	if got := d.next(t).(input.Key).Code; got != input.Up {
		t.Fatalf("event = %v, want the arrow key rather than an Escape", got)
	}
	d.silent(t, 60*time.Millisecond)
}

func TestPumpReportsResizes(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	defer close(d.stop)

	d.size = func() (int, int, error) { return 120, 40, nil }
	d.resized <- struct{}{}
	got := d.next(t).(input.Resize)
	if got.Width != 120 || got.Height != 40 {
		t.Fatalf("resize = %+v, want 120x40", got)
	}
}

func TestPumpIgnoresAResizeItCannotMeasure(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	defer close(d.stop)

	d.size = func() (int, int, error) { return 0, 0, errors.New("no size") }
	d.resized <- struct{}{}
	// Reporting a size of zero would have every widget lay out into nothing.
	d.silent(t, 40*time.Millisecond)
}

func TestPumpDeliversTheLastKeystrokeWhenInputEnds(t *testing.T) {
	d := newDriver(time.Second)
	// Bytes and the end of input arrive on separate channels, so both are waiting when
	// the pump comes round and a select cannot be told to prefer one. Neither the
	// keystroke that had not been decoded yet nor the one still held may be lost.
	d.raw <- []byte("ab\x1b")
	d.readErr <- errors.New("end of input")

	for _, want := range []rune{'a', 'b'} {
		if got := d.next(t).(input.Key); !got.IsRune(want, 0) {
			t.Fatalf("event = %+v, want %q", got, want)
		}
	}
	if got := d.next(t).(input.Key).Code; got != input.Esc {
		t.Fatalf("event = %v, want the held Escape", got)
	}
	select {
	case _, ok := <-d.events:
		if ok {
			t.Fatal("more events arrived after the input ended")
		}
	case <-time.After(time.Second):
		t.Fatal("the event channel never closed after the input ended")
	}
}

func TestPumpClosesItsChannelWhenAskedToStop(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	close(d.stop)
	select {
	case <-d.done:
	case <-time.After(time.Second):
		t.Fatal("the pump did not return")
	}
	if _, ok := <-d.events; ok {
		t.Fatal("the event channel was not closed")
	}
}
