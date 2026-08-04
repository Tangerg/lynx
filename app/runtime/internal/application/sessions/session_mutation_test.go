package sessions

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
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

	if err := newCoordinator(stores, turns).DeleteSession(t.Context(), "ses_1"); err != nil {
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

	err := newCoordinator(stores, turns).DeleteSession(t.Context(), "ses_1")
	if !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if slices.Contains(stores.operations, "turn.cancel") || slices.Contains(stores.operations, "session.forget") {
		t.Fatalf("operations after failure = %v, want no process cleanup", stores.operations)
	}
}

func TestDeleteSessionQuiescesGoalOnlyAfterDurableCommit(t *testing.T) {
	stores := newMutationStores("")
	coordinator := New(testDependencies(stores, Dependencies{
		Turns: mutationTurns{operations: &stores.operations},
		Paths: testCwdResolver{},
		Goals: mutationGoalGuard{operations: &stores.operations},
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	want := []string{"goal.mutation", "interrupts.list", "apply.delete", "goal.quiesce", "turn.cancel", "session.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestDeleteSessionDoesNotQuiesceGoalWhenDurableCommitFails(t *testing.T) {
	stores := newMutationStores("apply.delete")
	coordinator := New(testDependencies(stores, Dependencies{
		Turns: mutationTurns{operations: &stores.operations},
		Paths: testCwdResolver{},
		Goals: mutationGoalGuard{operations: &stores.operations},
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if slices.Contains(stores.operations, "goal.quiesce") {
		t.Fatalf("operations = %v, goal was quiesced after a failed write-set", stores.operations)
	}
}

func TestDeleteSessionCleansUpAfterGoalQuiesceFailure(t *testing.T) {
	quiesceErr := errors.New("goal quiesce failed")
	stores := newMutationStores("")
	coordinator := New(testDependencies(stores, Dependencies{
		Turns: mutationTurns{operations: &stores.operations},
		Paths: testCwdResolver{},
		Goals: mutationGoalGuard{operations: &stores.operations, quiesceErr: quiesceErr},
	}))

	err := coordinator.DeleteSession(t.Context(), "ses_1")
	if !errors.Is(err, quiesceErr) {
		t.Fatalf("DeleteSession error = %v, want quiesce failure", err)
	}
	want := []string{"goal.mutation", "interrupts.list", "apply.delete", "goal.quiesce", "turn.cancel", "session.forget"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want post-commit cleanup despite quiesce failure", stores.operations)
	}
}

func TestDeleteSessionDetachesParkedTurnCleanupFromCallerCancellation(t *testing.T) {
	stores := newMutationStores("")
	turns := new(observingTurns)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := newCoordinator(stores, turns).DeleteSession(ctx, "ses_1"); err != nil {
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

func TestDeleteSessionReportsEveryPostCommitCleanupFailure(t *testing.T) {
	turnErr := errors.New("turn cleanup failed")
	checkpointErr := errors.New("checkpoint cleanup failed")
	stores := newMutationStores("")
	checkpoints := &mutationCheckpoints{operations: &stores.operations, err: checkpointErr}
	coordinator := New(testDependencies(stores, Dependencies{
		Turns:       mutationTurns{operations: &stores.operations, err: turnErr},
		Paths:       testCwdResolver{},
		Checkpoints: checkpoints,
	}))

	err := coordinator.DeleteSession(t.Context(), "ses_1")
	if !errors.Is(err, turnErr) || !errors.Is(err, checkpointErr) {
		t.Fatalf("DeleteSession error = %v, want turn and checkpoint cleanup failures", err)
	}
	want := []string{"interrupts.list", "apply.delete", "turn.cancel", "session.forget", "checkpoint.drop:ses_1"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
	if len(stores.deleted) != 1 || stores.deleted[0] != "ses_1" {
		t.Fatal("cleanup failure prevented durable session deletion")
	}
}

func TestDeleteSessionDiscardsIsolatedSandboxCopyPostCommit(t *testing.T) {
	sandboxErr := errors.New("sandbox discard failed")
	stores := newMutationStores("")
	coordinator := New(testDependencies(stores, Dependencies{
		Turns:       mutationTurns{operations: &stores.operations},
		Paths:       testCwdResolver{},
		Checkpoints: &mutationCheckpoints{operations: &stores.operations},
		Sandbox:     &mutationSandbox{operations: &stores.operations, err: sandboxErr},
	}))

	err := coordinator.DeleteSession(t.Context(), "ses_1")
	if !errors.Is(err, sandboxErr) {
		t.Fatalf("DeleteSession error = %v, want sandbox discard failure surfaced", err)
	}
	// The sandbox copy is discarded post-commit, after the durable delete and the
	// checkpoint drop — never inside the write-set.
	want := []string{"interrupts.list", "apply.delete", "turn.cancel", "session.forget", "checkpoint.drop:ses_1", "sandbox.discard:ses_1"}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
	if len(stores.deleted) != 1 || stores.deleted[0] != "ses_1" {
		t.Fatal("sandbox cleanup failure prevented durable session deletion")
	}
}

func TestRollbackReportsParkedTurnCleanupFailure(t *testing.T) {
	turnErr := errors.New("turn cleanup failed")
	stores := newMutationStores("")
	coordinator := New(testDependencies(stores, Dependencies{
		Turns: mutationTurns{operations: &stores.operations, err: turnErr},
		Paths: testCwdResolver{},
	}))
	boundary := transcript.Boundary{Dropped: []transcript.RunNode{{ID: "run_1"}}}

	err := coordinator.applyRollback(t.Context(), "ses_1", boundary)
	if !errors.Is(err, turnErr) {
		t.Fatalf("applyRollback error = %v, want turn cleanup failure", err)
	}
	want := []string{
		"apply.rollback",
		"turn.cancel",
	}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestDeleteSessionAddressesOnlyTheRequestedConversation(t *testing.T) {
	stores := newMutationStores("")
	claims := new(testClaimer)

	if err := newCoordinatorWithAdmissions(stores, mutationTurns{operations: &stores.operations}, claims).DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	wantDeleted := []string{"ses_1"}
	if !slices.Equal(stores.deleted, wantDeleted) {
		t.Fatalf("deleted = %v, want %v", stores.deleted, wantDeleted)
	}
	if len(claims.claimed) != 0 || len(claims.released) != len(wantDeleted) {
		t.Fatalf("claims after delete = %+v releases=%v", claims.claimed, claims.released)
	}
}

// TestRestoreSessionAppliesPlan: RestoreSession forwards the decoded artifact to
// the atomic restore write-set verbatim.
func TestRestoreSessionAppliesPlan(t *testing.T) {
	stores := newMutationStores("")
	stores.pending = map[string][]interrupts.Pending{}
	_, err := newCoordinator(stores, mutationTurns{operations: &stores.operations}).restoreSession(
		t.Context(),
		Snapshot{
			Session:  session.Session{ID: "ses_1", Cwd: "/workspace"},
			Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hi"))},
		}, false,
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
	coordinator := New(testDependencies(stores, Dependencies{
		Turns: mutationTurns{operations: &stores.operations},
		Paths: testCwdResolver{err: want},
	}))

	_, err := coordinator.restoreSession(t.Context(), Snapshot{Session: session.Session{ID: "ses_1", Cwd: "relative"}}, false)
	if !errors.Is(err, session.ErrCwdUnavailable) || !errors.Is(err, want) {
		t.Fatalf("RestoreSession error = %v, want cwd unavailable + cause", err)
	}
	if len(stores.restored) != 0 {
		t.Fatalf("restore mutated storage after cwd rejection: %+v", stores.restored)
	}
}

var errMutationStage = errors.New("mutation stage failed")

type mutationGoalGuard struct {
	operations *[]string
	quiesceErr error
}

func (g mutationGoalGuard) WithSessionMutation(
	ctx context.Context,
	_ []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	*g.operations = append(*g.operations, "goal.mutation")
	if err := commit(ctx); err != nil {
		return err
	}
	*g.operations = append(*g.operations, "goal.quiesce")
	return errors.Join(g.quiesceErr, afterCommit(ctx))
}

// mutationStores supplies the coordinator's named persistence ports for mutation write-sets: it
// records the atomic Apply* calls + the process cleanup, and lists a single open
// interrupt so DeleteSession has a parked turn to cancel.
type mutationStores struct {
	operations []string
	fail       string
	deleted    []string
	restored   []RestorePlan
	ints       *mutationInterrupts
	pending    map[string][]interrupts.Pending
}

func newMutationStores(fail string) *mutationStores {
	s := &mutationStores{
		fail: fail,
		pending: map[string][]interrupts.Pending{
			"ses_1": {{
				RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
				Continuations: []interrupts.Continuation{{RunID: "run_1", ProcessID: "proc_1"}},
			}},
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

func (s *mutationStores) Session() Store                                       { return s }
func (s *mutationStores) Interrupts() InterruptStore                           { return s.ints }
func (s *mutationStores) Transcript() TranscriptStore                          { return emptyTranscript{} }
func (s *mutationStores) Runs() RunStore                                       { return emptyTranscript{} }
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
	s.deleted = append(s.deleted, plan.SessionID)
	return nil
}
func (s *mutationStores) ApplyTerminal(context.Context, TerminalPlan) error {
	return s.record("apply.cancel")
}

func (*mutationStores) List(context.Context) ([]session.Session, error) { panic("unused") }

func (*mutationStores) ListPage(context.Context, bool, int64, string, int) ([]session.Session, error) {
	panic("unused")
}
func (*mutationStores) Get(context.Context, string) (session.Session, error) { panic("unused") }
func (*mutationStores) Create(context.Context, string, string) (session.Session, error) {
	panic("unused")
}
func (*mutationStores) Ensure(context.Context, session.Session) (session.Session, error) {
	panic("unused")
}
func (*mutationStores) Patch(context.Context, string, session.Patch) (session.Session, error) {
	panic("unused")
}

type mutationInterrupts struct{ stores *mutationStores }

func (i *mutationInterrupts) Open(context.Context, interrupts.Pending) error { panic("unused") }
func (i *mutationInterrupts) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	if err := i.stores.record("interrupts.list"); err != nil {
		return nil, err
	}
	return i.stores.pending[sessionID], nil
}
func (i *mutationInterrupts) Get(_ context.Context, runID string) (interrupts.Pending, bool, error) {
	for _, pending := range i.stores.pending {
		for _, item := range pending {
			if item.RootRunID == runID {
				return item, true, nil
			}
		}
	}
	return interrupts.Pending{}, false, nil
}
func (i *mutationInterrupts) Consume(context.Context, string, string) (interrupts.Pending, bool, error) {
	panic("unused")
}

type mutationTurns struct {
	operations *[]string
	err        error
}

func (t mutationTurns) Cancel(context.Context, execution.TurnRef) error {
	*t.operations = append(*t.operations, "turn.cancel")
	return t.err
}

type observingTurns struct {
	calls    int
	canceled bool
	bounded  bool
}

func (t *observingTurns) Cancel(ctx context.Context, _ execution.TurnRef) error {
	t.calls++
	t.canceled = ctx.Err() != nil
	_, t.bounded = ctx.Deadline()
	return nil
}

type mutationCheckpoints struct {
	operations *[]string
	err        error
}

func (*mutationCheckpoints) Restore(context.Context, string, string, string) error {
	panic("unused")
}

func (c *mutationCheckpoints) DropSession(sessionID string) error {
	*c.operations = append(*c.operations, "checkpoint.drop:"+sessionID)
	return c.err
}

type mutationSandbox struct {
	operations *[]string
	err        error
}

func (s *mutationSandbox) Discard(sessionID string) error {
	*s.operations = append(*s.operations, "sandbox.discard:"+sessionID)
	return s.err
}
