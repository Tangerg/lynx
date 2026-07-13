package sessions

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type coordinatorStores struct {
	interrupts *coordinatorInterrupts
}

func (s coordinatorStores) Session() SessionStore       { panic("unused") }
func (s coordinatorStores) Interrupts() InterruptStore  { return s.interrupts }
func (s coordinatorStores) Transcript() TranscriptStore { return emptyTranscript{} }
func (s coordinatorStores) ReadHistory(context.Context, string) ([]chat.Message, error) {
	panic("unused")
}
func (s coordinatorStores) ForgetSession(string) {}
func (s coordinatorStores) ApplyFork(context.Context, ForkPlan) (session.Session, error) {
	panic("unused")
}

// The atomic write-sets delegate their interrupt drops to the interrupt fake so
// the coordinator tests observe them (the run-state transition an ApplyCancel /
// ApplyRollback also commits is verified at the sqlite/bootstrap level).
func (s coordinatorStores) ApplyRollback(ctx context.Context, plan RollbackPlan) error {
	for _, runID := range plan.DropRunIDs {
		_ = s.interrupts.Delete(ctx, runID)
	}
	return nil
}
func (s coordinatorStores) ApplyRestore(context.Context, RestorePlan) error { return nil }
func (s coordinatorStores) ApplyDelete(ctx context.Context, sessionID string) error {
	pending, _ := s.interrupts.List(ctx, sessionID)
	for _, p := range pending {
		_ = s.interrupts.Delete(ctx, p.RunID)
	}
	return nil
}
func (s coordinatorStores) ApplyCancel(ctx context.Context, _ string, runID string) error {
	return s.interrupts.Delete(ctx, runID)
}

type coordinatorInterrupts struct {
	pending  map[string]interrupts.Pending
	deleted  []string
	onDelete func(string)
}

func (s *coordinatorInterrupts) Put(_ context.Context, p interrupts.Pending) error {
	if s.pending == nil {
		s.pending = map[string]interrupts.Pending{}
	}
	s.pending[p.RunID] = p
	return nil
}

func (s *coordinatorInterrupts) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	out := make([]interrupts.Pending, 0, len(s.pending))
	for _, p := range s.pending {
		if sessionID == "" || p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *coordinatorInterrupts) Get(_ context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	p, ok := s.pending[parentRunID]
	return p, ok, nil
}

func (s *coordinatorInterrupts) Consume(_ context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	p, ok := s.pending[parentRunID]
	if ok {
		delete(s.pending, parentRunID)
	}
	return p, ok, nil
}

func (s *coordinatorInterrupts) Delete(_ context.Context, parentRunID string) error {
	s.deleted = append(s.deleted, parentRunID)
	if s.onDelete != nil {
		s.onDelete(parentRunID)
	}
	delete(s.pending, parentRunID)
	return nil
}

type testClaimer struct {
	claimed  map[string]bool
	released []string
}

func (c *testClaimer) ClaimSession(sessionID string) bool {
	if c.claimed == nil {
		c.claimed = map[string]bool{}
	}
	if c.claimed[sessionID] {
		return false
	}
	c.claimed[sessionID] = true
	return true
}

func (c *testClaimer) ReleaseSession(sessionID string) {
	c.released = append(c.released, sessionID)
	delete(c.claimed, sessionID)
}

// ActiveSessionWithCwd reports no cross-session working-tree contention by
// default; the file-rollback tests that need it drive a dedicated claimer.
func (*testClaimer) ActiveSessionWithCwd(string) string { return "" }

// newCoordinator builds a Coordinator over test stores and turns.
func newCoordinator(stores Stores, turns Turns) *Coordinator {
	return New(Dependencies{Stores: stores, Turns: turns, Paths: testCwdResolver{}})
}

type testCwdResolver struct {
	resolved string
	err      error
}

func (r testCwdResolver) ResolveExistingDir(path string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.resolved != "" {
		return r.resolved, nil
	}
	return path, nil
}

type emptyTranscript struct{}

func (emptyTranscript) List(context.Context, string) ([]transcript.Item, []transcript.Run, error) {
	return nil, nil, nil
}
