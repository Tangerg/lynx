// Package term owns the terminal itself: taking it over, giving it back, reading
// what it sends, and writing frames to it without blocking whoever drew them.
//
// It is the only package in the TUI that touches the operating system. Everything
// above it works in cells, events and frames, and can be tested without a terminal
// at all — which is the point of putting the parts that need one here and nowhere
// else.
package term

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"

	xterm "golang.org/x/term"

	"github.com/Tangerg/lynx/app/tui/primitives/input"
)

// ErrNotTerminal is reported by [Open] when the process is not attached to a
// terminal — piped, redirected, or running under something that gave it no tty.
var ErrNotTerminal = errors.New("term: not a terminal")

// Options says which of the terminal's optional behaviours a session wants.
//
// The zero value asks for none of them, which is a legitimate choice for a
// session that only wants raw keys, and is why each is named for what turning it
// on gets you rather than for turning it off.
type Options struct {
	// AltScreen draws on a screen of its own, leaving the user's scrollback as it
	// was. Without it, frames are drawn in place among whatever else is on screen.
	AltScreen bool
	// Mouse asks for mouse reporting, including movement and not only clicks.
	Mouse bool
	// Focus asks to be told when the terminal window gains or loses focus, which is
	// what lets a UI stop animating while nobody is looking.
	Focus bool
	// Keyboard asks for the Kitty keyboard protocol: unambiguous key codes, key
	// releases and repeats, and the text a key produced. Terminals that do not
	// implement it ignore the request.
	Keyboard bool
}

// Terminal is a terminal taken over for a session.
//
// It is the whole boundary: raw mode, the modes it turned on, the goroutines
// reading input, and the writer frames go through. [Terminal.Close] gives all of
// it back, in the order that leaves the terminal as it was found, and is safe to
// call more than once — including from a deferred call on a path that already
// failed.
type Terminal struct {
	in, out  *os.File
	modes    modes
	oldState *xterm.State

	events chan input.Event
	writer *Writer

	winch    chan os.Signal
	resized  chan struct{}
	stop     chan struct{}
	pumpDone chan struct{}
	fanDone  chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// Open takes over the terminal on standard input and output.
//
// It reports [ErrNotTerminal] when there is no terminal to take over, which is the
// case a caller has to handle rather than force: a program whose output is being
// piped wants to write text, not frames.
func Open(opts Options) (*Terminal, error) {
	in, out := os.Stdin, os.Stdout
	fd := int(in.Fd())
	if !xterm.IsTerminal(fd) {
		return nil, fmt.Errorf("%w: standard input is fd %d", ErrNotTerminal, fd)
	}

	// Raw mode first: it is what stops the terminal from interpreting keys on the
	// program's behalf, and everything below assumes it.
	oldState, err := xterm.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("term: enter raw mode: %w", err)
	}

	t := &Terminal{
		in:  in,
		out: out,
		modes: modes{
			altScreen: opts.AltScreen,
			mouse:     opts.Mouse,
			focus:     opts.Focus,
			keyboard:  opts.Keyboard,
		},
		oldState: oldState,
		events:   make(chan input.Event, 64),
		winch:    make(chan os.Signal, 1),
		resized:  make(chan struct{}, 1),
		stop:     make(chan struct{}),
		pumpDone: make(chan struct{}),
		fanDone:  make(chan struct{}),
	}

	if _, err := out.WriteString(t.modes.enter()); err != nil {
		_ = xterm.Restore(fd, oldState)
		return nil, fmt.Errorf("term: take over the terminal: %w", err)
	}

	t.writer = NewWriter(out)
	notifyResize(t.winch)

	// The size is delivered as an event rather than left to be asked for, so a
	// session learns its size the same way it learns about every later change.
	if w, h, err := t.Size(); err == nil {
		t.events <- input.Resize{Width: w, Height: h}
	}

	raw := make(chan []byte, 4)
	readErr := make(chan error, 1)
	p := &pump{
		raw: raw, readErr: readErr, resized: t.resized, stop: t.stop,
		out: t.events, size: t.Size,
	}
	go t.read(raw, readErr)
	go t.fanResize()
	go func() {
		defer close(t.pumpDone)
		p.run()
	}()
	return t, nil
}

// Events is the terminal's input, closed when the input ends or the session does.
func (t *Terminal) Events() <-chan input.Event { return t.events }

// Writer is where frames go.
func (t *Terminal) Writer() *Writer { return t.writer }

// Size is the terminal's size in cells.
func (t *Terminal) Size() (w, h int, err error) {
	return xterm.GetSize(int(t.out.Fd()))
}

// Close gives the terminal back.
//
// The order is the reverse of taking it over, and every step runs even if an
// earlier one failed: a terminal left in raw mode is unusable, so a failure to
// write the restore sequences must not be a reason to skip leaving raw mode.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		signal.Stop(t.winch)
		close(t.stop)
		<-t.fanDone

		// Frames first: the writer may still hold one, and writing it after the
		// modes have been put back would draw onto the user's restored screen.
		var errs []error
		if err := t.writer.Close(); err != nil {
			errs = append(errs, err)
		}
		if _, err := t.out.WriteString(t.modes.leave()); err != nil {
			errs = append(errs, fmt.Errorf("term: give the terminal back: %w", err))
		}
		if err := xterm.Restore(int(t.in.Fd()), t.oldState); err != nil {
			errs = append(errs, fmt.Errorf("term: leave raw mode: %w", err))
		}
		// The pump owns the event channel, so it has to have finished before anyone
		// could observe the channel closed.
		<-t.pumpDone
		t.closeErr = errors.Join(errs...)
	})
	return t.closeErr
}

// read performs the blocking reads and hands chunks to the pump.
//
// A blocking read on a terminal cannot be cancelled portably, so this goroutine
// outlives the request to stop and returns on the next byte or on end of input.
// Leaving raw mode, which Close does, is what makes that happen promptly in
// practice. Nothing waits for it: it holds no state anyone else needs.
func (t *Terminal) read(raw chan<- []byte, readErr chan<- error) {
	for {
		buf := make([]byte, 4096)
		n, err := t.in.Read(buf)
		if n > 0 {
			select {
			case raw <- buf[:n]:
			case <-t.stop:
				return
			}
		}
		if err != nil {
			readErr <- err
			return
		}
		select {
		case <-t.stop:
			return
		default:
		}
	}
}

// fanResize turns resize signals into wake-ups the pump can select on, collapsing
// a burst of them into one. A drag of the window edge produces dozens; the size is
// read once, after.
func (t *Terminal) fanResize() {
	defer close(t.fanDone)
	for {
		select {
		case <-t.winch:
			select {
			case t.resized <- struct{}{}:
			default:
			}
		case <-t.stop:
			return
		}
	}
}
