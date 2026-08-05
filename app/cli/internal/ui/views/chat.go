// Package views arranges parts into screens.
//
// A view owns a layout and the keys that belong to the screen as a whole, and it
// owns nothing else: it does not fetch, it does not fold events, and it does not
// draw anything itself. What it decides is where things go and who gets the keyboard,
// which is exactly the pair of decisions that has to be made in one place.
package views

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/ui/parts"
	"github.com/Tangerg/lynx/app/cli/internal/ui/store"
)

// Chat is the conversation screen: a transcript, a plan, whatever is being asked,
// and the field to write in.
//
// The vertical order is fixed and deliberate. The transcript takes what is left over,
// because it is the only part whose content has no natural size; everything else is
// as tall as it needs to be. What is being asked sits directly above the field,
// because that is where the eye already is when a prompt appears.
type Chat struct {
	// Send is called with a message the user submitted. It reports whether the message
	// was taken; a refusal leaves the text in the field rather than losing it.
	Send func(string) bool
	// Answer is called with the user's response to an approval.
	Answer func(client.Decision)
	// Cancel is called when the user asks to stop what is running.
	Cancel func()
	// Quit is called when the user asks to leave.
	Quit func()

	theme      kit.Theme
	transcript *parts.Transcript
	plan       *parts.Plan
	approval   *parts.Approval
	composer   *parts.Composer
	spinner    *kit.Spinner
	status     parts.Status
	help       kit.Help

	keys ChatKeys
	// session is the state last given to the view, held so drawing does not need it
	// passed in again.
	session *store.Session
}

// ChatKeys are the screen's own bindings, as opposed to the ones its parts own.
type ChatKeys struct {
	Quit    headless.Binding
	Cancel  headless.Binding
	Newline headless.Binding
}

// DefaultChatKeys are the bindings a terminal chat is expected to have.
func DefaultChatKeys() ChatKeys {
	return ChatKeys{
		Quit:   headless.Binding{Key: input.Key{Code: input.Character, Rune: 'd', Mods: input.Ctrl}, Does: "quit"},
		Cancel: headless.Binding{Key: input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, Does: "stop"},
		// Named so the hint row can say it, since it is the one keystroke a user
		// would not guess.
		Newline: headless.Binding{Key: input.Key{Code: input.Enter, Mods: input.Alt}, Does: "newline"},
	}
}

// NewChat builds the screen.
func NewChat(t kit.Theme) *Chat {
	c := &Chat{
		theme:      t,
		transcript: parts.NewTranscript(t),
		plan:       parts.NewPlan(t),
		approval:   parts.NewApproval(t),
		composer:   parts.NewComposer(t),
		spinner:    &kit.Spinner{Style: t.Accent},
		keys:       DefaultChatKeys(),
	}
	c.plan.Collapsed = true
	c.composer.Submit = func(body string) bool {
		if c.Send == nil {
			return false
		}
		return c.Send(body)
	}
	c.approval.Answer = func(d client.Decision) {
		if c.Answer != nil {
			c.Answer(d)
		}
	}
	c.status = parts.Status{Theme: t}
	c.help = kit.Help{
		KeyStyle:       t.Accent,
		DoesStyle:      t.Subtle,
		SeparatorStyle: t.Subtle,
	}
	return c
}

// Composer is the field, for putting text into it from outside — a resumed draft, a
// recipe, a command's arguments.
func (c *Chat) Composer() *parts.Composer { return c.composer }

// Tick advances anything animated.
func (c *Chat) Tick() { c.spinner.Tick() }

// Update gives the screen the state to draw.
//
// The approval prompt is opened and closed from here rather than by whoever sends the
// answer, so that what is on screen follows from the state and cannot disagree with
// it.
func (c *Chat) Update(s *store.Session) {
	c.session = s
	c.transcript.Update(s.Blocks, s.Revision())
	c.plan.Items = s.Plan
	c.status.Usage = s.Usage

	switch s.Phase() {
	case store.Waiting:
		if c.approval.Request().InterruptID != s.Approval.InterruptID {
			c.approval.Open(s.Approval)
		}
		c.status.Left = "waiting for you"
		c.status.Spinner = nil
	case store.Running:
		c.approval.Close()
		c.status.Left = "working"
		c.status.Spinner = c.spinner
	default:
		c.approval.Close()
		c.status.Left = c.idleStatus(s)
		c.status.Spinner = nil
	}
	c.help.Bindings = c.bindings()
}

// idleStatus says how the last run ended, or nothing at all before the first one.
func (c *Chat) idleStatus(s *store.Session) string {
	switch s.Outcome.Status {
	case client.OutcomeCompleted:
		return "done"
	case client.OutcomeCanceled:
		return "stopped"
	case client.OutcomeFailed:
		return "failed: " + s.Outcome.Error
	default:
		return ""
	}
}

// Handle routes an event to whatever should get it.
//
// The order is the whole of the screen's input policy, and each step is there for a
// reason:
//
// Leaving comes first, before anything at all. An open prompt takes the keyboard, and
// a prompt that could also swallow the way out would trap the user in front of a
// question they may not want to answer.
//
// The prompt comes next, because nothing else on the screen can be acted on until it
// is settled and what it is asking about is a change to the user's files.
//
// The field comes last, so it receives everything nobody else claimed: typing a "c" is
// typing a "c".
func (c *Chat) Handle(ev input.Event) bool {
	if c.keys.Quit.Matches(ev) {
		if c.Quit != nil {
			c.Quit()
		}
		return true
	}
	if c.approval.Handle(ev) {
		return true
	}
	switch {
	case c.keys.Cancel.Matches(ev):
		// Stop what is running; with nothing running, leave — which is what the same
		// keystroke does in a shell.
		if c.session != nil && c.session.Busy() {
			if c.Cancel != nil {
				c.Cancel()
			}
			return true
		}
		if c.Quit != nil {
			c.Quit()
		}
		return true
	}
	if c.composer.Handle(ev) {
		return true
	}
	return c.transcript.Handle(ev)
}

// band is one horizontal region of the screen and the widget that fills it.
//
// [layout.Rows] measures and arranges but deliberately does not draw, so the
// pairing is the caller's to keep. Holding both in one value is what stops a
// slot list and a draw list from drifting a row apart.
type band struct {
	widget headless.Widget
	size   layout.Sizing
}

// Draw lays the screen out.
func (c *Chat) Draw(v grid.View) {
	width, _ := v.Size()
	if width <= 0 {
		return
	}
	bands := []band{
		// The transcript absorbs whatever the others do not need.
		{c.transcript, layout.Flex(1)},
		{c.plan, layout.Measured(0, 8)},
		{c.approval, layout.Measured(0, 20)},
		{c.composer, layout.Measured(3, 10)},
		{c.status, layout.Fixed(1)},
		{c.help, layout.Fixed(1)},
	}
	slots := make([]layout.Slot, len(bands))
	for i, b := range bands {
		slots[i] = layout.Slot{Size: b.size}
		// A measured band is asked how much it wants; a fixed or flexible one is
		// told, and has nothing to answer.
		if sized, ok := b.widget.(headless.Sized); ok {
			slots[i].Of = sized
		}
	}
	for i, region := range layout.Rows(v, slots...) {
		bands[i].widget.Draw(region)
	}
}

// bindings are the hints for the current state, which is not the same as the list of
// keys that work: a hint for stopping a run that is not running is noise.
func (c *Chat) bindings() []headless.Binding {
	if c.approval.Showing() {
		return c.approval.Bindings()
	}
	out := []headless.Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send"},
		c.keys.Newline,
	}
	if c.session != nil && c.session.Busy() {
		out = append(out, c.keys.Cancel)
	}
	return append(out, c.keys.Quit)
}
