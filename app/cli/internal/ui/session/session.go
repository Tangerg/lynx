// Package session drives one conversation: it turns what the user does into calls on a
// runtime, and what the runtime says into what the screen shows.
//
// It is the seam between the interface library and this program. Everything above it —
// the screens, the widgets, the terminal — knows nothing about runs or approvals;
// everything below it knows nothing about terminals. This package is the only place
// that knows both, which is what keeps either side replaceable.
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/ui/store"
	"github.com/Tangerg/lynx/app/cli/internal/ui/views"
	"github.com/Tangerg/oolong/atoms/theme"
	"github.com/Tangerg/oolong/primitives/grid"
	"github.com/Tangerg/oolong/primitives/input"
	"github.com/Tangerg/oolong/primitives/term"
	"github.com/Tangerg/oolong/program"
)

// animationRate is how often something animated advances while something is animating.
const animationRate = 100 * time.Millisecond

// Config is what a conversation needs.
type Config struct {
	// Runtime is where the work happens.
	Runtime client.Runtime
	// Session is the session to open in. Empty creates one.
	Session string
	// Workspace is the directory a new session works in.
	Workspace string
	// Prompt is text to start the field with, so that a question typed on the command
	// line can be read and changed before it goes.
	Prompt string
	// Theme is the palette. Nil uses the dark one.
	Theme *theme.Theme
	// Host overrides where input comes from and frames go, for tests.
	Host program.Host
}

// Run opens the conversation and draws it until the user leaves.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Runtime == nil {
		return errors.New("session: a runtime is required")
	}
	opened, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	return program.Run(ctx, program.Config{
		Root: func(loop program.Loop) program.Component {
			return New(loop, cfg, opened)
		},
		Terminal: term.Options{Mouse: true, Focus: true, Keyboard: true},
		Host:     cfg.Host,
	})
}

// open finds or creates the session to work in.
func open(ctx context.Context, cfg Config) (client.Session, error) {
	if cfg.Session == "" {
		return cfg.Runtime.CreateSession(ctx, client.NewSession{Workspace: cfg.Workspace})
	}
	sessions, err := cfg.Runtime.ListSessions(ctx)
	if err != nil {
		return client.Session{}, err
	}
	for _, s := range sessions {
		if s.ID == cfg.Session {
			return s, nil
		}
	}
	return client.Session{}, fmt.Errorf("%w: %s", client.ErrSessionNotFound, cfg.Session)
}

// Conversation is the running conversation, and the component a program draws.
//
// Everything it holds is touched only on the program's goroutine. What arrives from
// elsewhere — a run's events — is posted there rather than applied where it landed,
// which is why none of this is guarded by a lock.
type Conversation struct {
	loop    program.Loop
	runtime client.Runtime
	session client.Session

	state *store.Session
	chat  *views.Chat

	// stream cancels whatever run is being followed.
	stream context.CancelFunc
	// following is set while a stream is being read, so a second message is refused
	// even before the runtime has answered the first.
	following bool
	// stopTick ends the animation clock. Nil when nothing is animating.
	stopTick func()
}

// New builds a conversation over a loop.
func New(loop program.Loop, cfg Config, opened client.Session) *Conversation {
	palette := theme.Dark()
	if cfg.Theme != nil {
		palette = *cfg.Theme
	}
	c := &Conversation{
		loop:    loop,
		runtime: cfg.Runtime,
		session: opened,
		state:   store.NewSession(),
		chat:    views.NewChat(palette),
	}
	c.chat.Send = c.send
	c.chat.Answer = c.answer
	c.chat.Cancel = c.cancel
	c.chat.Quit = loop.Quit
	if cfg.Prompt != "" {
		c.chat.Composer().Editor().SetText(cfg.Prompt)
	}
	return c
}

// Draw paints the conversation.
func (c *Conversation) Draw(v grid.View) {
	c.chat.Update(c.state)
	c.chat.Draw(v)
}

// Handle routes input to the screen.
func (c *Conversation) Handle(ev input.Event) bool { return c.chat.Handle(ev) }

// send starts a run for a message the user submitted, and reports whether the message
// was taken.
//
// It refuses while a run is going: the composer keeps the text, and the user is not
// left wondering where their message went.
func (c *Conversation) send(body string) bool {
	if c.state.Busy() || c.following {
		return false
	}
	c.state.Apply(client.BlockCompleted{Block: client.Block{
		ID: "local:" + body, Kind: client.BlockUser, Text: body,
	}})
	// Busy from this moment, not from whenever the runtime answers: the gap between
	// them can be a whole round trip, and a session that called itself idle in it would
	// read a request to stop as a request to quit.
	c.state.Starting()
	c.follow(func(ctx context.Context) (client.Stream, error) {
		return c.runtime.StartRun(ctx, client.StartRun{SessionID: c.session.ID, Prompt: body})
	})
	return true
}

// answer sends the user's response to an approval and follows the run that resumes.
func (c *Conversation) answer(d client.Decision) {
	request := c.state.Approval
	if request.InterruptID == "" {
		return
	}
	runID := c.state.RunID()
	c.state.Resumed()
	c.follow(func(ctx context.Context) (client.Stream, error) {
		return c.runtime.ResumeRun(ctx, client.ResumeRun{
			RunID:       runID,
			InterruptID: request.InterruptID,
			Decision:    d,
		})
	})
}

// cancel stops the run.
//
// A run that has not reported itself yet has no id to cancel by, and that window is as
// long as it takes the runtime to answer. Dropping the stream is the whole of it there,
// and the stop is recorded here because no stream is left to report it.
func (c *Conversation) cancel() {
	runID := c.state.RunID()
	if runID == "" {
		c.drop()
		c.apply(client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
		return
	}
	// With an id, the runtime is asked and the stream reports the stop. Nothing is
	// assumed here: a run that declines to stop is still running, and saying otherwise
	// would leave the interface disagreeing with the runtime.
	go func() {
		if err := c.runtime.CancelRun(context.WithoutCancel(context.Background()), runID); err != nil {
			c.loop.Post(func() { c.fail(err) })
		}
	}()
}

// follow opens a stream and reads it into the conversation.
//
// The goroutine only ever posts. It never touches what this type holds, which is what
// makes the program's goroutine the single owner and the whole interface safe without a
// lock.
func (c *Conversation) follow(open func(context.Context) (client.Stream, error)) {
	ctx, cancel := context.WithCancel(context.Background())
	c.drop()
	c.stream = cancel
	c.following = true
	c.animate(true)

	go func() {
		defer cancel()
		defer c.loop.Post(func() { c.following = false; c.animate(c.state.Busy()) })

		stream, err := open(ctx)
		if err != nil {
			c.loop.Post(func() { c.fail(err) })
			return
		}
		for ev, err := range stream {
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					c.loop.Post(func() { c.fail(err) })
				}
				return
			}
			c.loop.Post(func() { c.apply(ev) })
		}
	}()
}

// apply folds one event in and keeps the animation clock in step with it.
func (c *Conversation) apply(ev client.Event) {
	c.state.Apply(ev)
	c.animate(c.state.Phase() == store.Running)
}

// fail records that the run ended in a way the stream never got to report.
func (c *Conversation) fail(err error) {
	c.state.Failed(err)
	c.animate(false)
}

// drop cancels whatever stream is being read.
func (c *Conversation) drop() {
	if c.stream != nil {
		c.stream()
		c.stream = nil
	}
}

// animate starts or stops the clock that advances anything moving.
//
// Only while something is moving: a clock that ran regardless would wake the program up
// to redraw an interface nobody is waiting on.
func (c *Conversation) animate(on bool) {
	switch {
	case on && c.stopTick == nil:
		c.stopTick = c.loop.Every(animationRate, func() {
			c.chat.Tick()
		})
	case !on && c.stopTick != nil:
		c.stopTick()
		c.stopTick = nil
	}
}
