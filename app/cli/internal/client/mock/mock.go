package mock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// errCanceled marks a stop asked for through CancelRun, which ends a run with an
// outcome, as opposed to a context cancellation, which ends the call with an
// error.
var errCanceled = errors.New("mock: run canceled")

// Runtime is a scripted [client.Runtime]. It is safe for concurrent use: a
// stream is consumed on one goroutine while cancellation arrives on another,
// which is exactly how the CLI drives it.
type Runtime struct {
	// Instant drops every scripted delay. Tests set it; a human never wants it,
	// because the pacing is what makes streaming legible.
	Instant bool
	// Script builds the conversation a prompt gets. Nil uses [Conversation].
	Script func(prompt string) Script

	mu       sync.Mutex
	sessions []client.Session
	runs     map[string]*run
	next     int
}

// run is one scripted run's mutable state.
type run struct {
	id      string
	session string
	script  Script
	parked  bool
	// cancel closes once, on the first CancelRun.
	cancel chan struct{}
	once   sync.Once
}

// New builds a mock seeded with fake sessions.
func New() *Runtime {
	return &Runtime{
		sessions: demoSessions(),
		runs:     make(map[string]*run),
	}
}

var _ client.Runtime = (*Runtime)(nil)

// ListSessions returns the catalogue, most recently touched first.
func (r *Runtime) ListSessions(context.Context) ([]client.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.sessions)
	slices.SortStableFunc(out, func(a, b client.Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out, nil
}

// CreateSession opens a session, titling it from the workspace when the caller
// did not name it.
func (r *Runtime) CreateSession(_ context.Context, in client.NewSession) (client.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	title := in.Title
	if title == "" {
		title = "Untitled session"
	}
	s := client.Session{
		ID:        fmt.Sprintf("ses_mock_%d", r.next),
		Title:     title,
		Workspace: in.Workspace,
		UpdatedAt: time.Now(),
		Revision:  1,
	}
	r.sessions = append(r.sessions, s)
	return s, nil
}

// StartRun scripts a run against the prompt and returns its stream.
func (r *Runtime) StartRun(ctx context.Context, in client.StartRun) (client.Stream, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return nil, errors.New("mock: prompt is empty")
	}
	build := r.Script
	if build == nil {
		build = Conversation
	}

	r.mu.Lock()
	if !slices.ContainsFunc(r.sessions, func(s client.Session) bool { return s.ID == in.SessionID }) {
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	r.next++
	rn := &run{
		id:      fmt.Sprintf("run_mock_%d", r.next),
		session: in.SessionID,
		script:  build(in.Prompt),
		cancel:  make(chan struct{}),
	}
	r.runs[rn.id] = rn
	r.mu.Unlock()

	opening := []Step{{Event: client.RunStarted{RunID: rn.id, SessionID: rn.session}}}
	opening = append(opening, Step{Delay: beat, Event: client.BlockCompleted{Block: client.Block{
		ID: rn.id + "_prompt", Kind: client.BlockUser, Text: in.Prompt,
	}}})
	return r.play(ctx, rn, append(opening, rn.script.Prelude...), rn.script.parks()), nil
}

// ResumeRun answers the park and returns the continuation.
func (r *Runtime) ResumeRun(ctx context.Context, in client.ResumeRun) (client.Stream, error) {
	r.mu.Lock()
	rn, ok := r.runs[in.RunID]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", client.ErrRunNotFound, in.RunID)
	}
	if !rn.parked || in.InterruptID != rn.script.Approval.InterruptID {
		r.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", client.ErrInterruptNotOpen, in.InterruptID)
	}
	rn.parked = false
	steps := rn.script.Denied
	if in.Decision.Approved {
		steps = rn.script.Approved
	}
	r.mu.Unlock()
	return r.play(ctx, rn, steps, false), nil
}

// CancelRun lodges a stop. Repeat calls are no-ops, so a doubled Ctrl-C is not
// an error.
func (r *Runtime) CancelRun(_ context.Context, runID string) error {
	r.mu.Lock()
	rn, ok := r.runs[runID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s", client.ErrRunNotFound, runID)
	}
	rn.once.Do(func() { close(rn.cancel) })
	return nil
}

// play turns steps into a stream. The generator body runs on the consumer's
// goroutine — that is how pull iterators work — so a cancel arriving elsewhere
// is observed through rn.cancel rather than by interrupting this code.
func (r *Runtime) play(ctx context.Context, rn *run, steps []Step, park bool) client.Stream {
	return func(yield func(client.Event, error) bool) {
		for _, s := range steps {
			switch err := r.pause(ctx, rn, s.Delay); {
			case errors.Is(err, errCanceled):
				yield(client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}}, nil)
				return
			case err != nil:
				yield(nil, err)
				return
			}
			if !yield(s.Event, nil) {
				return
			}
		}
		if !park {
			return
		}
		r.mu.Lock()
		rn.parked = true
		approval := rn.script.Approval
		r.mu.Unlock()
		yield(client.RunParked{Approval: approval}, nil)
	}
}

// pause waits out one step's delay, reporting why it stopped early. Cancellation
// is checked even when there is nothing to wait for, so an instant script still
// honours a stop.
func (r *Runtime) pause(ctx context.Context, rn *run, d time.Duration) error {
	if r.Instant || d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-rn.cancel:
			return errCanceled
		default:
			return nil
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-rn.cancel:
		return errCanceled
	}
}
