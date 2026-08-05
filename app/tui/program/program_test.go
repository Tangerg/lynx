package program

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/tui/primitives/grid"
	"github.com/Tangerg/lynx/app/tui/primitives/input"
	"github.com/Tangerg/lynx/app/tui/primitives/term"
)

// Everything here is asserted against the bytes that reached the terminal, or against
// state the test itself owns behind a lock.
//
// Nothing reads what the component holds without going through the loop. That state
// belongs to the program's goroutine, and a test that reached into it would be a data
// race dressed up as a test — the very thing this package exists to make unnecessary.

// host stands in for a terminal: a channel of events in, a buffer of frames out.
type host struct {
	events chan input.Event
	frames *frames
	writer *term.Writer
	w, h   int
}

func newHost() *host {
	f := &frames{}
	return &host{
		events: make(chan input.Event, 64),
		frames: f,
		writer: term.NewWriter(f),
		w:      40,
		h:      10,
	}
}

func (h *host) Events() <-chan input.Event { return h.events }
func (h *host) Writer() *term.Writer       { return h.writer }
func (h *host) Size() (int, int, error)    { return h.w, h.h, nil }

func (h *host) send(ev input.Event) { h.events <- ev }
func (h *host) rune(r rune)         { h.send(input.Key{Code: input.Character, Rune: r}) }

// frames collects what reached the terminal.
type frames struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (f *frames) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.Write(p)
}

func (f *frames) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.String()
}

func (f *frames) size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.Len()
}

// component is a test component: it draws a line of text and counts what it was given.
//
// Its counters are atomic because a test reads them; the text it draws is only ever
// touched on the program's goroutine, which is the rule this package promises.
type component struct {
	loop Loop

	text    string
	handled atomic.Int64
	drawn   atomic.Int64
	// consume decides whether Handle claims what it is given.
	consume bool
}

func (c *component) Draw(v grid.View) {
	c.drawn.Add(1)
	v.Text(0, 0, c.text, grid.Style{})
}

func (c *component) Handle(ev input.Event) bool {
	c.handled.Add(1)
	if key, ok := ev.(input.Key); ok && key.Code == input.Character {
		c.text += string(key.Rune)
	}
	return c.consume
}

// running is a program under test.
type running struct {
	host *host
	root *component
	done chan error
	t    *testing.T
}

// start runs a program over a fake host.
func start(t *testing.T, prepare func(*component)) *running {
	t.Helper()
	h := newHost()
	root := &component{text: "ready", consume: true}
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Host: h,
			Root: func(loop Loop) Component {
				root.loop = loop
				if prepare != nil {
					prepare(root)
				}
				return root
			},
		})
	}()
	return &running{host: h, root: root, done: done, t: t}
}

// until waits for something to become true, failing if the program ends first.
func (r *running) until(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		select {
		case err := <-r.done:
			r.t.Fatalf("the program ended waiting for %s: %v", what, err)
		case <-time.After(2 * time.Millisecond):
		}
	}
	r.t.Fatalf("timed out waiting for %s", what)
}

func (r *running) wait() error {
	r.t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(10 * time.Second):
		r.t.Fatal("the program never returned")
		return nil
	}
}

func TestTheInterfaceIsDrawnBeforeAnythingHappens(t *testing.T) {
	// A program draws before it selects. Waiting for the user to press something would
	// leave a blank terminal in front of them.
	r := start(t, nil)
	r.until("the opening frame", func() bool {
		return strings.Contains(r.host.frames.String(), "ready")
	})
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestInputReachesTheComponent(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	r.host.rune('!')
	r.until("the keystroke to be handled", func() bool { return r.root.handled.Load() == 1 })
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestAnEventTheComponentDeclinesIsDropped(t *testing.T) {
	// A component is the root of its own tree. There is nobody above it to pass an
	// unclaimed event on to, and pretending otherwise would mean inventing a policy for
	// events nobody wanted.
	r := start(t, func(c *component) { c.consume = false })
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	r.host.rune('x')
	r.until("the keystroke to be offered", func() bool { return r.root.handled.Load() == 1 })
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestPostRunsOnTheProgramsGoroutineAndThenDraws(t *testing.T) {
	// The whole of the concurrency model: work that happened elsewhere is applied where
	// the state lives, and what it changed is shown.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

	done := make(chan struct{})
	r.root.loop.Post(func() {
		r.root.text = "posted"
		close(done)
	})
	<-done
	r.until("the posted change to be drawn", func() bool {
		return strings.Contains(r.host.frames.String(), "posted")
	})
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestRefreshAsksForAFrame(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	settled := r.root.drawn.Load()
	r.root.loop.Refresh()
	r.until("another frame", func() bool { return r.root.drawn.Load() > settled })
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestEveryTicksUntilItIsStopped(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

	var ticks atomic.Int64
	stop := r.root.loop.Every(5*time.Millisecond, func() { ticks.Add(1) })
	r.until("the clock to tick", func() bool { return ticks.Load() >= 3 })

	stop()
	time.Sleep(30 * time.Millisecond)
	settled := ticks.Load()
	time.Sleep(60 * time.Millisecond)
	if grown := ticks.Load() - settled; grown != 0 {
		t.Fatalf("the clock ticked %d more times after being stopped", grown)
	}
	// Stopping twice is not an error: an owner that stops a clock on more than one path
	// should not have to remember which one ran.
	stop()

	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestEveryStopsWhenTheProgramDoes(t *testing.T) {
	// A clock nobody stopped must not outlive the program it was drawing for.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

	var ticks atomic.Int64
	r.root.loop.Every(5*time.Millisecond, func() { ticks.Add(1) })
	r.until("the clock to tick", func() bool { return ticks.Load() >= 2 })

	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	settled := ticks.Load()
	time.Sleep(60 * time.Millisecond)
	if grown := ticks.Load() - settled; grown != 0 {
		t.Fatalf("the clock ticked %d more times after the program ended", grown)
	}
}

func TestAnIntervalOfNothingIsNotAClock(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

	var ticks atomic.Int64
	stop := r.root.loop.Every(0, func() { ticks.Add(1) })
	stop()
	if r.root.loop.Every(time.Millisecond, nil) == nil {
		t.Fatal("a clock with nothing to call returned no way to stop it")
	}
	time.Sleep(20 * time.Millisecond)
	if ticks.Load() != 0 {
		t.Fatal("a clock with no interval ticked anyway")
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestPostAfterTheProgramHasStoppedIsDropped(t *testing.T) {
	// Blocking a caller for ever would be worse than losing work there is nothing left
	// to show.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	loop := r.root.loop
	loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}

	settled := make(chan struct{})
	go func() {
		defer close(settled)
		for range 100 {
			loop.Post(func() {})
		}
		loop.Refresh()
	}()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("posting to a program that has stopped blocked the caller")
	}
}

func TestTheInputEndingEndsTheProgram(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	close(r.host.events)
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestACancelledContextEndsTheProgramWithoutAnError(t *testing.T) {
	// Being asked to stop is not a failure.
	h := newHost()
	root := &component{text: "ready", consume: true}
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- Run(ctx, Config{Host: h, Root: func(Loop) Component { return root }})
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the program ignored the cancellation")
	}
}

func TestAResizeChangesTheGeometryAndRepaints(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	r.host.send(input.Resize{Width: 20, Height: 4})
	// A full repaint at the new size positions the cursor on a row only the new geometry
	// has.
	r.until("a frame at the new size", func() bool {
		return strings.Contains(r.host.frames.String(), "\x1b[4;")
	})
	// It is the program's own event: the component is never offered it, because it has
	// nothing to decide about it.
	if got := r.root.handled.Load(); got != 0 {
		t.Fatalf("the component was offered %d events, want none", got)
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestRegainingFocusRepaints(t *testing.T) {
	// Another program may have written to the terminal while this one was not in front,
	// so what it is showing can no longer be assumed.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	settled := r.host.frames.size()
	r.host.send(input.FocusIn{})
	r.until("a repaint", func() bool { return r.host.frames.size() > settled })
	if got := r.root.handled.Load(); got != 0 {
		t.Fatalf("the component was offered %d events, want none", got)
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestAnIdleProgramStopsWriting(t *testing.T) {
	// The reason the presenter exists: a terminal written to on every pass is a terminal
	// whose cursor never blinks and whose remote session never settles.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	time.Sleep(150 * time.Millisecond)
	settled := r.host.frames.size()
	time.Sleep(400 * time.Millisecond)
	if grown := r.host.frames.size() - settled; grown != 0 {
		t.Fatalf("an idle program wrote %d more bytes", grown)
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestABurstOfUpdatesIsCoalescedIntoFewerFrames(t *testing.T) {
	// A stream of updates arriving faster than a terminal can be redrawn must not become
	// a frame each, or the interface spends its time drawing instead of keeping up.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	before := r.root.drawn.Load()

	const updates = 200
	settled := make(chan struct{})
	for i := range updates {
		r.root.loop.Post(func() {
			r.root.text = strings.Repeat(".", i%20)
			if i == updates-1 {
				close(settled)
			}
		})
	}
	<-settled
	time.Sleep(100 * time.Millisecond)
	if frames := r.root.drawn.Load() - before; frames >= updates {
		t.Fatalf("%d updates produced %d frames, want them coalesced", updates, frames)
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestTheLastUpdateOfABurstIsStillDrawn(t *testing.T) {
	// Coalescing must not lose the tail: a frame turned away for arriving too soon has
	// to come due, or the final state of a stream is never shown.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	for range 20 {
		r.root.loop.Post(func() { r.root.text = "middle" })
	}
	r.root.loop.Post(func() { r.root.text = "final" })
	r.until("the last update to be drawn", func() bool {
		return strings.Contains(r.host.frames.String(), "final")
	})
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestAFailedTerminalEndsTheProgramWithItsError(t *testing.T) {
	// An interface that cannot reach its terminal has nothing left to do, and a loop
	// that kept going would spin on the same error for ever.
	h := newHost()
	h.writer = term.NewWriter(brokenTerminal{})
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Host: h,
			Root: func(Loop) Component { return &component{text: "ready"} },
		})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errBrokenTerminal) {
			t.Fatalf("program returned %v, want the write failure", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the program kept going after the terminal failed")
	}
}

func TestAProgramWithNoComponentIsRefused(t *testing.T) {
	if err := Run(context.Background(), Config{Host: newHost()}); err == nil {
		t.Fatal("a program with nothing to draw was accepted")
	}
}

func TestTheComponentIsGivenItsLoopBeforeItIsDrawn(t *testing.T) {
	// So that a component can hand the loop to whatever fetches on its behalf from the
	// moment it exists, rather than having to wait for a first frame.
	h := newHost()
	var hadLoop atomic.Bool
	done := make(chan error, 1)
	var quit Loop
	go func() {
		done <- Run(context.Background(), Config{
			Host: h,
			Root: func(loop Loop) Component {
				quit = loop
				hadLoop.Store(loop != nil)
				return &component{text: "ready"}
			},
		})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !hadLoop.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !hadLoop.Load() {
		t.Fatal("the component was built without a loop")
	}
	quit.Quit()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the program never returned")
	}
}

// brokenTerminal fails every write.
type brokenTerminal struct{}

var errBrokenTerminal = errors.New("the terminal went away")

func (brokenTerminal) Write([]byte) (int, error) { return 0, errBrokenTerminal }
