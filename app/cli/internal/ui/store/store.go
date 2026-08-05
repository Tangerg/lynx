// Package store folds a run's events into what a screen displays.
//
// The events arrive one at a time and say what changed; a screen needs to know what
// is, all at once, every frame. This package is the one place that turns the first
// into the second.
//
// There is exactly one such place on purpose. A transcript that folded events itself
// and a status bar that folded them again would eventually disagree about what
// happened, and the version on screen would be whichever one drew last.
package store

import (
	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// Phase is what a session is doing, as far as the screen is concerned.
type Phase uint8

const (
	// Idle is waiting for the user.
	Idle Phase = iota
	// Running is working, with nothing needed from the user.
	Running
	// Waiting is parked on an approval and going nowhere until it is answered.
	Waiting
)

// Session is everything one screen of one session needs.
//
// It is read by whatever draws and written only by [Session.Apply], so a frame
// always shows a state that some sequence of events actually produced.
type Session struct {
	// Blocks is the transcript in order.
	Blocks []client.Block
	// Plan is the run's plan, empty when there is none.
	Plan []client.PlanItem
	// Usage is what the last finished run consumed.
	Usage client.Usage
	// Approval is what the run is parked on, when it is.
	Approval client.Approval
	// Outcome is how the last run ended.
	Outcome client.Outcome

	phase Phase
	runID string
	// index finds a block by id, so a delta arriving for a block halfway up a long
	// transcript does not mean walking it.
	index map[string]int
	// revision counts changes, so a renderer can tell whether anything happened
	// without comparing what it drew last time.
	revision uint64
}

// NewSession returns an empty session.
func NewSession() *Session {
	return &Session{index: make(map[string]int)}
}

// Phase is what the session is doing.
func (s *Session) Phase() Phase { return s.phase }

// RunID is the run in progress, or the last one, or empty before the first.
func (s *Session) RunID() string { return s.runID }

// Revision counts the changes applied. It is how a loop decides whether to draw.
func (s *Session) Revision() uint64 { return s.revision }

// Busy reports whether the session has a run that has not finished — running or
// parked. It is one question with one answer, rather than two comparisons every
// caller has to remember to make together.
func (s *Session) Busy() bool { return s.phase != Idle }

// Apply folds one event in.
//
// An event about a block nobody started is dropped rather than guessed at: it would
// mean the stream and this state have diverged, and inventing a block to hang it on
// would hide that rather than show it.
func (s *Session) Apply(ev client.Event) {
	if s.index == nil {
		s.index = make(map[string]int)
	}
	switch e := ev.(type) {
	case client.RunStarted:
		s.runID = e.RunID
		s.phase = Running
		s.Outcome = client.Outcome{}
		s.Approval = client.Approval{}
	case client.BlockStarted:
		s.append(e.Block)
	case client.BlockDelta:
		at, ok := s.index[e.BlockID]
		if !ok {
			return
		}
		s.Blocks[at].Text += e.Text
	case client.BlockCompleted:
		if at, ok := s.index[e.Block.ID]; ok {
			s.Blocks[at] = e.Block
			break
		}
		// A block that completed without ever having started is legitimate: a whole
		// item that never streamed arrives once, complete.
		s.append(e.Block)
	case client.PlanChanged:
		s.Plan = e.Items
	case client.RunParked:
		s.phase = Waiting
		s.Approval = e.Approval
	case client.RunFinished:
		s.phase = Idle
		s.Approval = client.Approval{}
		s.Outcome = e.Outcome
		s.Usage = e.Usage
	default:
		return
	}
	s.revision++
}

// Starting records that a run has been asked for, before the stream has said anything
// about it.
//
// From the user's point of view the session is busy the moment they press send, and the
// gap before the runtime answers can be a whole network round trip. A session that
// called itself idle in that gap would answer a request to stop by quitting the
// program, which is the worst possible reading of the same keystroke.
func (s *Session) Starting() {
	s.phase = Running
	s.Outcome = client.Outcome{}
	s.Approval = client.Approval{}
	s.revision++
}

// Resumed records that the parked run is going again, which the store cannot learn
// from the stream: the answer to an approval is sent, not received.
func (s *Session) Resumed() {
	if s.phase != Waiting {
		return
	}
	s.phase = Running
	s.Approval = client.Approval{}
	s.revision++
}

// Failed records that the run ended in a way the stream never got to report — a
// connection lost, a call refused. The screen has to show something, and showing
// nothing would leave a spinner turning forever.
func (s *Session) Failed(err error) {
	s.phase = Idle
	s.Approval = client.Approval{}
	s.Outcome = client.Outcome{Status: client.OutcomeFailed, Error: err.Error()}
	s.Blocks = append(s.Blocks, client.Block{
		ID:   "failure",
		Kind: client.BlockError,
		Text: err.Error(),
	})
	s.revision++
}

// Reset empties the session, for switching to another one.
func (s *Session) Reset() {
	*s = Session{index: make(map[string]int), revision: s.revision + 1}
}

func (s *Session) append(b client.Block) {
	if at, ok := s.index[b.ID]; ok {
		s.Blocks[at] = b
		return
	}
	s.index[b.ID] = len(s.Blocks)
	s.Blocks = append(s.Blocks, b)
}
