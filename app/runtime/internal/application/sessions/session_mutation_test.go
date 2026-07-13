package sessions

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// TestDeleteSessionAppliesThenCleansUpProcesses: DeleteSession reads the open
// interrupts, commits the atomic delete write-set, then tears down the parked
// turns and the resume gate — in that order (the durable state is gone before the
// process-local cleanup).
func TestDeleteSessionAppliesThenCleansUpProcesses(t *testing.T) {
	stores := newMutationStores("")
	turns := mutationTurns{operations: &stores.operations}

	if err := newCoordinator(stores, turns).DeleteSession(context.Background(), "ses_1"); err != nil {
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

	err := newCoordinator(stores, turns).DeleteSession(context.Background(), "ses_1")
	if !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if slices.Contains(stores.operations, "turn.cancel") || slices.Contains(stores.operations, "session.forget") {
		t.Fatalf("operations after failure = %v, want no process cleanup", stores.operations)
	}
}

// TestRestoreSessionAppliesPlan: RestoreSession forwards the decoded artifact to
// the atomic restore write-set verbatim.
func TestRestoreSessionAppliesPlan(t *testing.T) {
	stores := newMutationStores("")
	err := newCoordinator(stores, mutationTurns{operations: &stores.operations}).RestoreSession(
		context.Background(),
		session.Session{ID: "ses_1"},
		[]chat.Message{chat.NewUserMessage("hi")},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if len(stores.restored) != 1 || stores.restored[0].Session.ID != "ses_1" || len(stores.restored[0].Messages) != 1 {
		t.Fatalf("restored = %+v, want one plan for ses_1 with 1 message", stores.restored)
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
}

func newMutationStores(fail string) *mutationStores {
	s := &mutationStores{fail: fail}
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

func (s *mutationStores) Session() SessionStore       { panic("unused") }
func (s *mutationStores) Interrupts() InterruptStore  { return s.ints }
func (s *mutationStores) Transcript() TranscriptStore { return emptyTranscript{} }
func (*mutationStores) ReadHistory(context.Context, string) ([]chat.Message, error) {
	panic("unused")
}
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
func (s *mutationStores) ApplyDelete(_ context.Context, sessionID string) error {
	if err := s.record("apply.delete"); err != nil {
		return err
	}
	s.deleted = append(s.deleted, sessionID)
	return nil
}
func (s *mutationStores) ApplyCancel(context.Context, string, string) error {
	return s.record("apply.cancel")
}

type mutationInterrupts struct{ stores *mutationStores }

func (i *mutationInterrupts) Put(context.Context, interrupts.Pending) error { panic("unused") }
func (i *mutationInterrupts) List(context.Context, string) ([]interrupts.Pending, error) {
	if err := i.stores.record("interrupts.list"); err != nil {
		return nil, err
	}
	return []interrupts.Pending{{RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"}}, nil
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
