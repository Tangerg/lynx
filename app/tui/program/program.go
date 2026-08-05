// Package program runs a terminal interface.
//
// It owns the terminal, the frame schedule, and the one goroutine that is allowed to
// touch the interface's state. It knows nothing about what the interface is for: what
// it drives is a [Component], which draws itself and answers input, and everything a
// component needs from the program it asks for through a [Loop].
//
// # The concurrency model, in full
//
// One goroutine draws and handles input. Anything that happens elsewhere — a request
// finishing, a file changing, a timer firing — reaches the interface by being posted
// to that goroutine with [Loop.Post], and runs there. That is the whole of it, and it
// is why every widget below this package is an ordinary mutable object with no lock in
// it.
//
// The program parks when there is nothing to do. It wakes for input, for posted work,
// and for the terminal reporting progress — never on a clock that runs regardless. A
// component that wants a clock starts one with [Loop.Every], and an interface with
// nothing animating costs nothing.
package program

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
	"github.com/Tangerg/lynx/app/tui/primitives/present"
	"github.com/Tangerg/lynx/app/tui/primitives/term"
)

// DefaultFrameRate is the fastest a program redraws. A terminal cannot usefully show
// more, and a stream of updates would otherwise ask for a frame each.
const DefaultFrameRate = 16 * time.Millisecond

// Component is an interface a program can run: it draws itself into the space it is
// given, and says whether it wants an event.
//
// It is handed a view that is already positioned and clipped, so its coordinates are
// its own. An event it does not consume is dropped by the program — a component is the
// root of its own tree and there is nobody above it to pass one on to.
type Component interface {
	Draw(v grid.View)
	Handle(ev input.Event) bool
}

// Loop is what a component may ask of the program running it.
//
// Every method is safe from any goroutine, which is the point: a component holds one of
// these and hands it to whatever fetches, watches or waits on its behalf, and the
// answers come back on the goroutine that owns the state.
type Loop interface {
	// Refresh asks for a frame.
	Refresh()

	// Post runs fn on the program's goroutine and asks for a frame afterwards.
	//
	// This is the only safe way to change what a component holds from anywhere else.
	// Work posted after the program has stopped is dropped rather than run: there is
	// nothing left to show it, and blocking the caller for ever would be worse.
	Post(fn func())

	// Every calls fn on the program's goroutine at an interval until the returned
	// function is called, or until the program stops.
	//
	// It is how anything animated advances. Nothing ticks unless something asked for
	// it, which is what lets an idle interface be silent.
	Every(d time.Duration, fn func()) (stop func())

	// Quit asks the program to stop. The program returns from Run once the frame in
	// hand has been dealt with.
	Quit()
}

// Host is where a program's input comes from and its frames go.
//
// A program opens the real terminal unless it is given one of these. Being able to
// supply it is what lets an interface be driven and inspected in a test, with no
// terminal in sight.
type Host interface {
	// Events is the input, closed when the input ends.
	Events() <-chan input.Event
	// Writer is where frames go.
	Writer() *term.Writer
	// Size is the terminal's size in cells.
	Size() (w, h int, err error)
}

// Config is what a program needs to run.
type Config struct {
	// Root builds the component to run. It is given the loop first, so the component
	// can hold it from the moment it exists.
	//
	// Required.
	Root func(Loop) Component

	// Terminal says which of the terminal's optional behaviours to ask for. Ignored
	// when Host is set.
	Terminal term.Options

	// Host overrides where input comes from and frames go. Nil opens the real terminal
	// and gives it back on the way out.
	Host Host

	// FrameRate caps how often the interface redraws. Zero uses [DefaultFrameRate].
	FrameRate time.Duration
}

// Run draws the interface until it is asked to stop, its input ends, or the terminal
// fails.
//
// A cancelled context stops the program without being reported as a failure: being
// asked to stop is not one.
func Run(ctx context.Context, cfg Config) (err error) {
	if cfg.Root == nil {
		return errors.New("program: a root component is required")
	}
	host := cfg.Host
	if host == nil {
		terminal, openErr := term.Open(cfg.Terminal)
		if openErr != nil {
			return openErr
		}
		// Giving the terminal back matters more than anything that could go wrong while
		// using it: a terminal left in raw mode is one the user has to close.
		defer func() { err = errors.Join(err, terminal.Close()) }()
		host = terminal
	}

	// The size is asked for once, here, rather than waited for. A program draws before
	// it selects — that is what puts the interface up without the user having to press
	// something — and a first frame drawn onto a screen of no size is a blank terminal.
	width, height, err := host.Size()
	if err != nil {
		return err
	}

	p := &program{
		host:      host,
		screen:    grid.NewScreen(width, height),
		writer:    host.Writer(),
		frameRate: cfg.FrameRate,
		tasks:     make(chan func(), 256),
		done:      make(chan struct{}),
	}
	if p.frameRate <= 0 {
		p.frameRate = DefaultFrameRate
	}
	defer close(p.done)
	p.root = cfg.Root(loop{p})
	return p.run(ctx)
}

// program is one running interface.
type program struct {
	root   Component
	host   Host
	screen *grid.Screen
	writer *term.Writer

	present   present.Presenter
	frameRate time.Duration

	// tasks carries work to be run on this goroutine. Its buffer is what absorbs a
	// burst without making the producer wait.
	tasks chan func()
	// done closes when the program stops, so anything waiting to post gives up and
	// anything ticking exits.
	done chan struct{}

	quit bool
}

// run is the event loop.
func (p *program) run(ctx context.Context) error {
	events := p.host.Events()

	// due fires when a frame that was turned away for arriving too soon becomes
	// allowed. Without it the last update of a burst would sit undrawn until something
	// else happened to wake the loop.
	due := time.NewTimer(0)
	if !due.Stop() {
		<-due.C
	}
	armed := false

	p.present.RequestFull()
	for !p.quit {
		p.draw()

		if armed && !due.Stop() {
			<-due.C
		}
		armed = false
		if at, pending := p.present.DueAt(); pending {
			due.Reset(max(time.Until(at), 0))
			armed = true
		}

		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-events:
			if !ok {
				// The input has ended, which is the session ending: a terminal that went
				// away, a pipe that closed.
				return nil
			}
			p.handle(ev)

		case task := <-p.tasks:
			p.apply(task)
			// Whatever else arrived without waiting is applied before drawing, so a
			// burst is one frame rather than one frame each.
			p.drain()

		case <-p.writer.Progress():
			p.present.Wrote(p.writer.Written())
			if err := p.writer.Err(); err != nil {
				// A terminal that has failed a write does not recover, and an interface
				// that cannot reach its terminal has nothing left to do.
				return err
			}

		case <-due.C:
			armed = false
		}
	}
	return nil
}

// apply runs one posted task and asks for a frame.
func (p *program) apply(task func()) {
	if task != nil {
		task()
	}
	p.present.RequestBy(time.Now(), p.frameRate)
}

// drain runs whatever else is already waiting.
func (p *program) drain() {
	for {
		select {
		case task := <-p.tasks:
			p.apply(task)
		default:
			return
		}
	}
}

// handle deals with one terminal event.
//
// Resize and focus are the program's own: the first changes the geometry everything
// else is drawn against, and the second means another program may have written to the
// terminal, so what it is showing can no longer be assumed. Everything else is the
// component's.
func (p *program) handle(ev input.Event) {
	switch e := ev.(type) {
	case input.Resize:
		p.screen.Resize(e.Width, e.Height)
		p.present.RequestFull()
		return
	case input.FocusIn:
		p.present.RequestFull()
		return
	}
	if p.root.Handle(ev) {
		p.present.RequestBy(time.Now(), p.frameRate)
	}
}

// draw renders a frame, if one is owed and the terminal is keeping up.
func (p *program) draw() {
	p.present.Present(time.Now(), func(full bool) uint64 {
		if full {
			p.screen.Invalidate()
		}
		p.root.Draw(p.screen.Frame())
		return p.flush()
	})
}

// flush hands the frame to the writer and returns the sequence it was queued under.
//
// The screen writes into a buffer rather than straight to the terminal, because the
// write has to happen on the writer's goroutine: that is the whole reason there is one.
func (p *program) flush() uint64 {
	var frame frameBuffer
	// Nothing here can fail — the destination is memory — so the error is the compiler's
	// concern and not this program's.
	_ = p.screen.Flush(&frame)
	if len(frame.bytes) == 0 {
		return 0
	}
	return p.writer.Queue(frame.bytes)
}

// frameBuffer collects one frame's bytes.
type frameBuffer struct{ bytes []byte }

func (f *frameBuffer) Write(b []byte) (int, error) {
	f.bytes = append(f.bytes, b...)
	return len(b), nil
}

// loop is the program's side of [Loop]. It is a value so a component can copy it
// freely and hand it to whatever needs it.
type loop struct{ p *program }

func (l loop) Refresh() { l.Post(nil) }

func (l loop) Post(fn func()) {
	select {
	case l.p.tasks <- fn:
	case <-l.p.done:
		// The program has stopped. There is nothing left to show the work on, and
		// blocking the caller for ever would be worse than dropping it.
	}
}

func (l loop) Quit() {
	l.Post(func() { l.p.quit = true })
}

func (l loop) Every(d time.Duration, fn func()) (stop func()) {
	if d <= 0 || fn == nil {
		return func() {}
	}
	stopped := make(chan struct{})
	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.Post(fn)
			case <-stopped:
				return
			case <-l.p.done:
				return
			}
		}
	}()
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		close(stopped)
	}
}
