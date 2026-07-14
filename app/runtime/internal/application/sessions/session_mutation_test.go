package sessions

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// TestDeleteSessionAppliesThenCleansUpProcesses: DeleteSession reads the open
// interrupts, commits the atomic delete write-set, then tears down the parked
// turns and the resume gate — in that order (the durable state is gone before the
// process-local cleanup).
func TestDeleteSessionAppliesThenCleansUpProcesses(t *testing.T) {
	stores := newMutationStores("")
	turns := mutationTurns{operations: &stores.operations}

	if err := newCoordinator(stores, turns).DeleteSession(t.Context(), new(testClaimer), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	want := []string{"interrupts.list", "apply.delete", "turn.cancel", "session.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
	if len(stores.deleted) != 1 || stores.deleted[0] != "ses_1" {
		t.Fatalf("deleted = %v, want [ses_1]", stores.deleted)
	}
}

// TestDeleteSessionStopsBeforeProcessCleanupOnApplyFailure: a failed write-set
// leaves the parked turns and the resume gate untouched (no half-cleanup on a
// durable failure).
func TestDeleteSessionStopsBeforeProcessCleanupOnApplyFailure(t *testing.T) {
	stores := newMutationStores("apply.delete")
	turns := mutationTurns{operations: &stores.operations}

	err := newCoordinator(stores, turns).DeleteSession(t.Context(), new(testClaimer), "ses_1")
	if !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if slices.Contains(stores.operations, "turn.cancel") || slices.Contains(stores.operations, "session.forget") {
		t.Fatalf("operations after failure = %v, want no process cleanup", stores.operations)
	}
}

func TestDeleteSessionDetachesParkedTurnCleanupFromCallerCancellation(t *testing.T) {
	stores := newMutationStores("")
	turns := new(observingTurns)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := newCoordinator(stores, turns).DeleteSession(ctx, new(testClaimer), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if turns.calls != 1 {
		t.Fatalf("turn Cancel calls = %d, want 1", turns.calls)
	}
	if turns.canceled {
		t.Fatal("turn cleanup inherited caller cancellation")
	}
	if !turns.bounded {
		t.Fatal("turn cleanup context has no deadline")
	}
}

func TestDeleteSessionRemovesOwnedSubtaskTreeButPreservesUserForks(t *testing.T) {
	stores := newMutationStores("")
	stores.children = map[string][]session.Session{
		"ses_1": {
			{ID: "ses_sub", Kind: session.KindSubtask},
			{ID: "ses_fork"},
		},
		"ses_sub": {{ID: "ses_nested", Kind: session.KindSubtask}},
	}
	stores.pending["ses_sub"] = []interrupts.Pending{{RunID: "run_sub", SessionID: "ses_sub", TurnID: "turn_sub"}}
	claims := new(testClaimer)

	if err := newCoordinator(stores, mutationTurns{operations: &stores.operations}).DeleteSession(t.Context(), claims, "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	wantDeleted := []string{"ses_nested", "ses_sub", "ses_1"}
	if !slices.Equal(stores.deleted, wantDeleted) {
		t.Fatalf("deleted = %v, want %v (user fork preserved)", stores.deleted, wantDeleted)
	}
	if len(claims.claimed) != 0 || len(claims.released) != len(wantDeleted) {
		t.Fatalf("claims after delete = %+v releases=%v", claims.claimed, claims.released)
	}
}

func TestDeleteSessionRejectsActiveSubtaskDescendant(t *testing.T) {
	stores := newMutationStores("")
	stores.children = map[string][]session.Session{
		"ses_1": {{ID: "ses_sub", Kind: session.KindSubtask}},
	}
	claims := &testClaimer{claimed: map[string]bool{"ses_sub": true}}

	err := newCoordinator(stores, mutationTurns{operations: &stores.operations}).DeleteSession(t.Context(), claims, "ses_1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("DeleteSession error = %v, want ErrSessionBusy", err)
	}
	if slices.Contains(stores.operations, "apply.delete") {
		t.Fatal("active descendant allowed the delete write-set to commit")
	}
	if claims.claimed["ses_1"] {
		t.Fatal("failed subtree claim leaked the root admission")
	}
}

func TestRollbackRejectsActiveSubtaskDescendantBeforeWriteSet(t *testing.T) {
	stores := newMutationStores("")
	stores.children = map[string][]session.Session{
		"ses_1": {{ID: "ses_sub", Kind: session.KindSubtask}},
	}
	claims := &testClaimer{claimed: map[string]bool{"ses_sub": true}}
	boundary := transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_drop"}}}

	_, _, err := newCoordinator(stores, mutationTurns{operations: &stores.operations}).prepareRollbackSessions(t.Context(), claims, "ses_1", boundary)
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Rollback error = %v, want ErrSessionBusy", err)
	}
	if slices.Contains(stores.operations, "apply.rollback") {
		t.Fatal("active subtask descendant allowed the rollback write-set to commit")
	}
}

// TestRestoreSessionAppliesPlan: RestoreSession forwards the decoded artifact to
// the atomic restore write-set verbatim.
func TestRestoreSessionAppliesPlan(t *testing.T) {
	stores := newMutationStores("")
	stores.pending = map[string][]interrupts.Pending{}
	err := newCoordinator(stores, mutationTurns{operations: &stores.operations}).RestoreSession(
		t.Context(),
		new(testClaimer),
		session.Session{ID: "ses_1", Cwd: "/workspace"},
		[]chat.Message{chat.NewUserMessage(chat.NewTextPart("hi"))},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(stores.restored) != 1 || stores.restored[0].Session.ID != "ses_1" || len(stores.restored[0].Messages) != 1 {
		t.Fatalf("restored = %+v, want one plan for ses_1 with 1 message", stores.restored)
	}
}

func TestRestoreSessionRejectsUnresolvableCwdBeforeMutation(t *testing.T) {
	stores := newMutationStores("")
	stores.pending = map[string][]interrupts.Pending{}
	want := errors.New("missing workspace")
	coordinator := New(Dependencies{
		Stores: stores,
		Turns:  mutationTurns{operations: &stores.operations},
		Paths:  testCwdResolver{err: want},
	})

	err := coordinator.RestoreSession(
		t.Context(), new(testClaimer), session.Session{ID: "ses_1", Cwd: "relative"}, nil, nil, nil,
	)
	if !errors.Is(err, session.ErrCwdUnavailable) || !errors.Is(err, want) {
		t.Fatalf("RestoreSession error = %v, want cwd unavailable + cause", err)
	}
	if len(stores.restored) != 0 {
		t.Fatalf("restore mutated storage after cwd rejection: %+v", stores.restored)
	}
}

var errMutationStage = errors.New("mutation stage failed")

// mutationStores is the coordinator's Stores view for the mutation write-sets: it
// records the atomic Apply* calls + the process cleanup, and lists a single open
// interrupt so DeleteSession has a parked turn to cancel.
type mutationStores struct {
	operations []string
	fail       string
	deleted    []string
	restored   []RestorePlan
	ints       *mutationInterrupts
	children   map[string][]session.Session
	pending    map[string][]interrupts.Pending
}

func newMutationStores(fail string) *mutationStores {
	s := &mutationStores{
		fail: fail,
		pending: map[string][]interrupts.Pending{
			"ses_1": {{RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"}},
		},
	}
	s.ints = &mutationInterrupts{stores: s}
	return s
}

func (s *mutationStores) record(stage string) error {
	s.operations = append(s.operations, stage)
	if s.fail == stage {
		return errMutationStage
	}
	return nil
}

func (s *mutationStores) Session() SessionStore                                { return s }
func (s *mutationStores) Interrupts() InterruptStore                           { return s.ints }
func (s *mutationStores) Transcript() TranscriptStore                          { return emptyTranscript{} }
func (*mutationStores) ReadSnapshot(context.Context, string) (Snapshot, error) { panic("unused") }
func (s *mutationStores) ForgetSession(string) {
	s.operations = append(s.operations, "session.forget")
}
func (*mutationStores) ApplyFork(context.Context, ForkPlan) (session.Session, error) {
	panic("unused")
}

func (s *mutationStores) ApplyRollback(context.Context, RollbackPlan) error {
	return s.record("apply.rollback")
}
func (s *mutationStores) ApplyRestore(_ context.Context, plan RestorePlan) error {
	if err := s.record("apply.restore"); err != nil {
		return err
	}
	s.restored = append(s.restored, plan)
	return nil
}
func (s *mutationStores) ApplyDelete(_ context.Context, plan DeletePlan) error {
	if err := s.record("apply.delete"); err != nil {
		return err
	}
	s.deleted = append(s.deleted, plan.SessionIDs...)
	return nil
}
func (s *mutationStores) ApplyCancel(context.Context, CancelPlan) error {
	return s.record("apply.cancel")
}

func (*mutationStores) List(context.Context) ([]session.Session, error)      { panic("unused") }
func (*mutationStores) Get(context.Context, string) (session.Session, error) { panic("unused") }
func (*mutationStores) Create(context.Context, string, string) (session.Session, error) {
	panic("unused")
}
func (*mutationStores) Patch(context.Context, string, session.Patch) (session.Session, error) {
	panic("unused")
}
func (s *mutationStores) Children(_ context.Context, parentID string) ([]session.Session, error) {
	return s.children[parentID], nil
}

type mutationInterrupts struct{ stores *mutationStores }

func (i *mutationInterrupts) Put(context.Context, interrupts.Pending) error { panic("unused") }
func (i *mutationInterrupts) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	if err := i.stores.record("interrupts.list"); err != nil {
		return nil, err
	}
	return i.stores.pending[sessionID], nil
}
func (i *mutationInterrupts) Get(context.Context, string) (interrupts.Pending, bool, error) {
	panic("unused")
}
func (i *mutationInterrupts) Consume(context.Context, string) (interrupts.Pending, bool, error) {
	panic("unused")
}

type mutationTurns struct{ operations *[]string }

func (t mutationTurns) Cancel(context.Context, RunRef) error {
	*t.operations = append(*t.operations, "turn.cancel")
	return nil
}

type observingTurns struct {
	calls    int
	canceled bool
	bounded  bool
}

func (t *observingTurns) Cancel(ctx context.Context, _ RunRef) error {
	t.calls++
	t.canceled = ctx.Err() != nil
	_, t.bounded = ctx.Deadline()
	return nil
}
