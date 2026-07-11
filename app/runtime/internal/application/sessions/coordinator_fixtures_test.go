package sessions

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

type coordinatorStores struct {
	interrupts *coordinatorInterrupts
}

func (s coordinatorStores) Session() SessionStore       { panic("unused") }
func (s coordinatorStores) Transcript() TranscriptStore { panic("unused") }
func (s coordinatorStores) Interrupts() InterruptStore  { return s.interrupts }
func (s coordinatorStores) ReadHistory(context.Context, string) ([]chat.Message, error) {
	panic("unused")
}
func (s coordinatorStores) TruncateMessages(context.Context, string, int) error { panic("unused") }
func (s coordinatorStores) SeedHistory(context.Context, string, []chat.Message) error {
	panic("unused")
}
func (s coordinatorStores) ForgetSession(string) {}
func (s coordinatorStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
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
	s.pending[p.ParentRunID] = p
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

// stubTurns is the test double for the coordinator's [Turns] collaborator: the
// combined cancel / resume / rehydrate surface. Each behavior is optional so a
// test wires only the leg it exercises.
type stubTurns struct {
	onCancel        func(RunRef)
	resumeErr       error
	rehydrateErr    error
	rehydrateHandle Handle
	onResume        func(RunRef, interrupts.Resolution, []string)
	onRehydrate     func(RehydrateSpec)
}

func (t stubTurns) Cancel(_ context.Context, ref RunRef) error {
	if t.onCancel != nil {
		t.onCancel(ref)
	}
	return nil
}

func (t stubTurns) Resume(_ context.Context, ref RunRef, r interrupts.Resolution, interruptKinds []string) (Handle, error) {
	if t.onResume != nil {
		t.onResume(ref, r, interruptKinds)
	}
	return turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}, t.resumeErr
}

func (t stubTurns) Rehydrate(_ context.Context, req RehydrateSpec) (Handle, error) {
	if t.onRehydrate != nil {
		t.onRehydrate(req)
	}
	return t.rehydrateHandle, t.rehydrateErr
}

// fakeDurableRuns records the durable-admission writes the abandonment
// write-sets make, so a test can assert a canceled/rolled-back/deleted run frees
// its session's durable slot.
type fakeDurableRuns struct {
	terminalized []string
	deleted      []string
}

func (f *fakeDurableRuns) Terminalize(_ context.Context, sessionID, _ string) error {
	f.terminalized = append(f.terminalized, sessionID)
	return nil
}

func (f *fakeDurableRuns) DeleteForSession(_ context.Context, sessionID string) error {
	f.deleted = append(f.deleted, sessionID)
	return nil
}

// newCoordinator builds a Coordinator over test stores and turns, without a
// durable run-admission backstop (in-process-only).
func newCoordinator(stores Stores, turns Turns) *Coordinator {
	return New(Dependencies{Stores: stores, Turns: turns})
}

// newCoordinatorWithRuns builds a Coordinator wired to a durable run-admission
// backstop so a test can observe the slot-freeing writes.
func newCoordinatorWithRuns(stores Stores, turns Turns, runs DurableRuns) *Coordinator {
	return New(Dependencies{Stores: stores, Turns: turns, Runs: runs})
}
