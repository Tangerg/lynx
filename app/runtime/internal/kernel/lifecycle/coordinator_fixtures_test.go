package lifecycle

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
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

type cancelTurns struct {
	onCancel func(turn.TurnHandle)
}

func (t cancelTurns) Cancel(_ context.Context, h turn.TurnHandle) error {
	if t.onCancel != nil {
		t.onCancel(h)
	}
	return nil
}

type resumeTurns struct {
	resumeErr       error
	rehydrateErr    error
	rehydrateHandle turn.TurnHandle
	onResume        func(turn.TurnHandle, interrupts.Resolution, []string)
	onRehydrate     func(turn.RehydrateRequest)
}

func (t resumeTurns) Resume(_ context.Context, h turn.TurnHandle, r interrupts.Resolution, interruptKinds []string) error {
	if t.onResume != nil {
		t.onResume(h, r, interruptKinds)
	}
	return t.resumeErr
}

func (t resumeTurns) Rehydrate(_ context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	if t.onRehydrate != nil {
		t.onRehydrate(req)
	}
	return t.rehydrateHandle, t.rehydrateErr
}
