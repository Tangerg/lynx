package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func TestResolveRollbackBoundary(t *testing.T) {
	t0 := time.Unix(10, 0)
	nodes := []transcript.RunNode{
		{ID: "run_3", CreatedAt: t0.Add(3 * time.Second), Mark: 9},
		{ID: "run_1", CreatedAt: t0, Mark: 3},
		{ID: "run_2", CreatedAt: t0.Add(time.Second), Mark: 6},
		{ID: "run_2_resume", ParentRunID: "run_2", CreatedAt: t0.Add(2 * time.Second), Mark: 7},
	}

	b, err := ResolveRollbackBoundary(nodes, "run_1")
	if err != nil {
		t.Fatalf("resolve rollback boundary: %v", err)
	}
	if b.KeepMark != 3 {
		t.Fatalf("KeepMark = %d, want 3", b.KeepMark)
	}
	wantDrop := []string{"run_2", "run_2_resume", "run_3"}
	if len(b.DropRunIDs) != len(wantDrop) {
		t.Fatalf("DropRunIDs = %v, want %v", b.DropRunIDs, wantDrop)
	}
	for i, want := range wantDrop {
		if b.DropRunIDs[i] != want {
			t.Fatalf("DropRunIDs = %v, want %v", b.DropRunIDs, wantDrop)
		}
	}
	if !b.BoundaryTime.Equal(t0.Add(time.Second)) {
		t.Fatalf("BoundaryTime = %v, want first dropped root time", b.BoundaryTime)
	}
}

func TestResolveForkHistoryPrefix(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage("one"),
		chat.NewAssistantMessage("two"),
		chat.NewUserMessage("three"),
	}
	nodes := []transcript.RunNode{
		{ID: "run_1", CreatedAt: time.Unix(1, 0), Mark: 1},
		{ID: "run_1_resume", ParentRunID: "run_1", CreatedAt: time.Unix(2, 0), Mark: 2},
		{ID: "run_2", CreatedAt: time.Unix(3, 0), Mark: 3},
	}

	got, err := ResolveForkHistoryPrefix(msgs, nodes, "run_1_resume")
	if err != nil {
		t.Fatalf("resolve fork prefix: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("prefix len = %d, want 2", len(got))
	}
}

func TestResolveForkHistoryPrefixKeepsFullHistoryOnUnknownMark(t *testing.T) {
	msgs := []chat.Message{chat.NewUserMessage("one"), chat.NewAssistantMessage("two")}
	nodes := []transcript.RunNode{{ID: "run_1", CreatedAt: time.Unix(1, 0), Mark: -1}}

	got, err := ResolveForkHistoryPrefix(msgs, nodes, "run_1")
	if err != nil {
		t.Fatalf("resolve fork prefix: %v", err)
	}
	if len(got) != len(msgs) {
		t.Fatalf("prefix len = %d, want full history %d", len(got), len(msgs))
	}
}

func TestCancelParkedRunCancelsTurnBeforeDeletingInterrupt(t *testing.T) {
	var order []string
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
			onDelete: func(string) { order = append(order, "delete") },
		},
	}
	turns := cancelTurns{onCancel: func(h turn.TurnHandle) {
		order = append(order, "cancel")
		if h.SessionID != "ses_1" || h.TurnID != "turn_1" {
			t.Fatalf("handle = %+v, want ses_1/turn_1", h)
		}
	}}

	if err := New(stores).CancelParkedRun(context.Background(), turns, "run_1"); err != nil {
		t.Fatalf("cancel parked run: %v", err)
	}
	if got := stores.interrupts.deleted; len(got) != 1 || got[0] != "run_1" {
		t.Fatalf("deleted = %v, want [run_1]", got)
	}
	if len(order) != 2 || order[0] != "cancel" || order[1] != "delete" {
		t.Fatalf("order = %v, want cancel then delete", order)
	}
}

func TestCancelParkedRunMissing(t *testing.T) {
	stores := cancelStores{interrupts: &cancelInterrupts{pending: map[string]interrupts.Pending{}}}
	err := New(stores).CancelParkedRun(context.Background(), cancelTurns{}, "missing")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound", err)
	}
	if len(stores.interrupts.deleted) != 0 {
		t.Fatalf("deleted = %v, want none", stores.interrupts.deleted)
	}
}

func TestClaimRunSlotHoldsAndReleasesSession(t *testing.T) {
	stores := cancelStores{interrupts: &cancelInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{}

	admission, err := New(stores).ClaimRunSlot(context.Background(), claimer, "ses_1")
	if err != nil {
		t.Fatalf("claim run slot: %v", err)
	}
	if admission.SessionID != "ses_1" {
		t.Fatalf("admission session = %q, want ses_1", admission.SessionID)
	}
	if !claimer.claimed["ses_1"] {
		t.Fatal("session should be claimed")
	}

	admission.Release()
	if claimer.claimed["ses_1"] {
		t.Fatal("session should be released")
	}
	if len(claimer.released) != 1 || claimer.released[0] != "ses_1" {
		t.Fatalf("released = %v, want [ses_1]", claimer.released)
	}
}

func TestClaimRunSlotRejectsOpenInterrupt(t *testing.T) {
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1"},
			},
		},
	}
	claimer := &testClaimer{}

	_, err := New(stores).ClaimRunSlot(context.Background(), claimer, "ses_1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("err = %v, want ErrSessionBusy", err)
	}
	if claimer.claimed["ses_1"] {
		t.Fatal("failed admission must release its claim")
	}
	if len(claimer.released) != 1 || claimer.released[0] != "ses_1" {
		t.Fatalf("released = %v, want [ses_1]", claimer.released)
	}
}

func TestClaimRunSlotRejectsActiveClaim(t *testing.T) {
	stores := cancelStores{interrupts: &cancelInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{claimed: map[string]bool{"ses_1": true}}

	_, err := New(stores).ClaimRunSlot(context.Background(), claimer, "ses_1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("err = %v, want ErrSessionBusy", err)
	}
	if len(claimer.released) != 0 {
		t.Fatalf("released = %v, want none", claimer.released)
	}
}

func TestClaimMutationSlotAllowsOpenInterrupt(t *testing.T) {
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1"},
			},
		},
	}
	claimer := &testClaimer{}

	admission, err := New(stores).ClaimMutationSlot(claimer, "ses_1")
	if err != nil {
		t.Fatalf("claim mutation slot: %v", err)
	}
	if admission.SessionID != "ses_1" || !claimer.claimed["ses_1"] {
		t.Fatalf("admission = %+v claimed = %v, want ses_1 claimed", admission, claimer.claimed)
	}
	admission.Release()
}

func TestClaimResumeSlotPeeksAndClaimsInterruptSession(t *testing.T) {
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
		},
	}
	claimer := &testClaimer{}

	pending, admission, err := New(stores).ClaimResumeSlot(context.Background(), claimer, "run_1")
	if err != nil {
		t.Fatalf("claim resume slot: %v", err)
	}
	if pending.ParentRunID != "run_1" || pending.SessionID != "ses_1" {
		t.Fatalf("pending = %+v, want run_1/ses_1", pending)
	}
	if admission.SessionID != "ses_1" || !claimer.claimed["ses_1"] {
		t.Fatalf("admission = %+v claimed = %v, want ses_1 claimed", admission, claimer.claimed)
	}

	admission.Release()
	if claimer.claimed["ses_1"] {
		t.Fatal("resume admission should release the session")
	}
}

func TestClaimResumeSlotMissingInterrupt(t *testing.T) {
	stores := cancelStores{interrupts: &cancelInterrupts{pending: map[string]interrupts.Pending{}}}
	claimer := &testClaimer{}

	_, _, err := New(stores).ClaimResumeSlot(context.Background(), claimer, "missing")
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("err = %v, want ErrInterruptNotOpen", err)
	}
	if len(claimer.claimed) != 0 {
		t.Fatalf("claimed = %v, want none", claimer.claimed)
	}
}

func TestResumeClaimedInterruptConsumesAndResumes(t *testing.T) {
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
		},
	}
	resolution := interrupts.Resolution{Approved: true}
	turns := resumeTurns{onResume: func(h turn.TurnHandle, got interrupts.Resolution) {
		if h.SessionID != "ses_1" || h.TurnID != "turn_1" {
			t.Fatalf("handle = %+v, want ses_1/turn_1", h)
		}
		if !got.Approved {
			t.Fatalf("resolution = %+v, want approved", got)
		}
	}}

	resumed, err := New(stores).ResumeClaimedInterrupt(context.Background(), turns, "run_1", resolution)
	if err != nil {
		t.Fatalf("resume claimed interrupt: %v", err)
	}
	if resumed.Pending.ParentRunID != "run_1" || resumed.Handle.TurnID != "turn_1" {
		t.Fatalf("resumed = %+v", resumed)
	}
	if _, ok := stores.interrupts.pending["run_1"]; ok {
		t.Fatal("interrupt must be consumed")
	}
}

func TestResumeClaimedInterruptRehydratesMissingTurn(t *testing.T) {
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {
					ParentRunID: "run_1",
					SessionID:   "ses_1",
					TurnID:      "turn_1",
					ProcessID:   "proc_1",
					Provider:    "anthropic",
					Model:       "claude",
				},
			},
		},
	}
	turns := resumeTurns{
		resumeErr:       turn.ErrTurnNotFound,
		rehydrateHandle: turn.TurnHandle{SessionID: "ses_1", TurnID: "turn_rebuilt"},
		onRehydrate: func(req turn.RehydrateRequest) {
			if req.SessionID != "ses_1" || req.ProcessID != "proc_1" || !req.Approved || req.Provider != "anthropic" || req.Model != "claude" {
				t.Fatalf("rehydrate request = %+v", req)
			}
		},
	}

	resumed, err := New(stores).ResumeClaimedInterrupt(context.Background(), turns, "run_1", interrupts.Resolution{Approved: true})
	if err != nil {
		t.Fatalf("resume claimed interrupt: %v", err)
	}
	if resumed.Handle.TurnID != "turn_rebuilt" {
		t.Fatalf("handle = %+v, want rebuilt turn", resumed.Handle)
	}
}

func TestResumeClaimedInterruptParkClaimed(t *testing.T) {
	stores := cancelStores{
		interrupts: &cancelInterrupts{
			pending: map[string]interrupts.Pending{
				"run_1": {ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"},
			},
		},
	}
	turns := resumeTurns{resumeErr: turn.ErrParkClaimed}

	_, err := New(stores).ResumeClaimedInterrupt(context.Background(), turns, "run_1", interrupts.Resolution{Approved: true})
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("err = %v, want ErrInterruptNotOpen", err)
	}
}

type cancelStores struct {
	interrupts *cancelInterrupts
}

func (s cancelStores) Session() session.Store       { panic("unused") }
func (s cancelStores) Transcript() transcript.Store { panic("unused") }
func (s cancelStores) Interrupts() interrupts.Store { return s.interrupts }
func (s cancelStores) ReadHistory(context.Context, string) ([]chat.Message, error) {
	panic("unused")
}
func (s cancelStores) TruncateMessages(context.Context, string, int) error { panic("unused") }
func (s cancelStores) SeedHistory(context.Context, string, []chat.Message) error {
	panic("unused")
}
func (s cancelStores) ForgetSession(string) {}
func (s cancelStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type cancelInterrupts struct {
	pending  map[string]interrupts.Pending
	deleted  []string
	onDelete func(string)
}

func (s *cancelInterrupts) Put(_ context.Context, p interrupts.Pending) error {
	if s.pending == nil {
		s.pending = map[string]interrupts.Pending{}
	}
	s.pending[p.ParentRunID] = p
	return nil
}

func (s *cancelInterrupts) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	out := make([]interrupts.Pending, 0, len(s.pending))
	for _, p := range s.pending {
		if sessionID == "" || p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *cancelInterrupts) Get(_ context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	p, ok := s.pending[parentRunID]
	return p, ok, nil
}

func (s *cancelInterrupts) Consume(_ context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	p, ok := s.pending[parentRunID]
	if ok {
		delete(s.pending, parentRunID)
	}
	return p, ok, nil
}

func (s *cancelInterrupts) Delete(_ context.Context, parentRunID string) error {
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
	rehydrateHandle turn.TurnHandle
	onResume        func(turn.TurnHandle, interrupts.Resolution)
	onRehydrate     func(turn.RehydrateRequest)
}

func (t resumeTurns) Resume(_ context.Context, h turn.TurnHandle, r interrupts.Resolution) error {
	if t.onResume != nil {
		t.onResume(h, r)
	}
	return t.resumeErr
}

func (t resumeTurns) Rehydrate(_ context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	if t.onRehydrate != nil {
		t.onRehydrate(req)
	}
	return t.rehydrateHandle, nil
}
