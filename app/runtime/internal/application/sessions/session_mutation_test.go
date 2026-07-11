package sessions

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func TestDeleteSessionCommitsDurableStateBeforeProcessCleanup(t *testing.T) {
	stores := newDeleteStores("")
	turns := deleteTurns{operations: &stores.operations}

	if err := newCoordinator(stores, turns).DeleteSession(context.Background(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	want := []string{
		"tx.begin",
		"interrupts.list",
		"transcript.delete",
		"messages.truncate",
		"interrupt.delete",
		"session.delete",
		"tx.commit",
		"turn.cancel",
		"session.forget",
	}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestDeleteSessionDropsDurableRunRowsInTransaction(t *testing.T) {
	stores := newDeleteStores("")
	turns := deleteTurns{operations: &stores.operations}
	runs := deleteRunsRecorder{stores: stores}

	if err := newCoordinatorWithRuns(stores, turns, runs).DeleteSession(context.Background(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// The durable run rows drop inside the same transaction as the rest of the
	// cascade (between the interrupt delete and the session delete), so a deleted
	// session leaves no admission row keeping it durably busy.
	want := []string{
		"tx.begin",
		"interrupts.list",
		"transcript.delete",
		"messages.truncate",
		"interrupt.delete",
		"runs.delete",
		"session.delete",
		"tx.commit",
		"turn.cancel",
		"session.forget",
	}
	if !slices.Equal(stores.operations, want) {
		t.Fatalf("operations = %v, want %v", stores.operations, want)
	}
}

func TestDeleteSessionStopsBeforeProcessCleanupOnDurableFailure(t *testing.T) {
	tests := []struct {
		name string
		fail string
	}{
		{name: "interrupt list", fail: "interrupts.list"},
		{name: "transcript", fail: "transcript.delete"},
		{name: "messages", fail: "messages.truncate"},
		{name: "interrupt delete", fail: "interrupt.delete"},
		{name: "session", fail: "session.delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stores := newDeleteStores(tt.fail)
			turns := deleteTurns{operations: &stores.operations}

			err := newCoordinator(stores, turns).DeleteSession(context.Background(), "ses_1")
			if !errors.Is(err, errDeleteStage) {
				t.Fatalf("DeleteSession error = %v, want %v", err, errDeleteStage)
			}
			if slices.Contains(stores.operations, "tx.commit") || slices.Contains(stores.operations, "turn.cancel") || slices.Contains(stores.operations, "session.forget") {
				t.Fatalf("operations after failure = %v", stores.operations)
			}
			if !slices.Contains(stores.operations, "tx.rollback") {
				t.Fatalf("operations = %v, want rollback", stores.operations)
			}
		})
	}
}

var errDeleteStage = errors.New("delete stage failed")

type deleteStores struct {
	operations []string
	fail       string
	session    deleteSessionStore
	transcript deleteTranscriptStore
	interrupts deleteInterruptStore
}

func newDeleteStores(fail string) *deleteStores {
	stores := &deleteStores{fail: fail}
	stores.session = deleteSessionStore{stores: stores}
	stores.transcript = deleteTranscriptStore{stores: stores}
	stores.interrupts = deleteInterruptStore{stores: stores}
	return stores
}

func (s *deleteStores) record(stage string) error {
	s.operations = append(s.operations, stage)
	if s.fail == stage {
		return errDeleteStage
	}
	return nil
}

func (s *deleteStores) Session() SessionStore       { return s.session }
func (s *deleteStores) Transcript() TranscriptStore { return s.transcript }
func (s *deleteStores) Interrupts() InterruptStore  { return s.interrupts }
func (*deleteStores) ReadHistory(context.Context, string) ([]chat.Message, error) {
	panic("unused")
}
func (s *deleteStores) TruncateMessages(context.Context, string, int) error {
	return s.record("messages.truncate")
}
func (*deleteStores) SeedHistory(context.Context, string, []chat.Message) error { panic("unused") }
func (s *deleteStores) ForgetSession(string) {
	s.operations = append(s.operations, "session.forget")
}
func (s *deleteStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	s.operations = append(s.operations, "tx.begin")
	if err := fn(ctx); err != nil {
		s.operations = append(s.operations, "tx.rollback")
		return err
	}
	s.operations = append(s.operations, "tx.commit")
	return nil
}

type deleteSessionStore struct{ stores *deleteStores }

func (deleteSessionStore) List(context.Context) ([]session.Session, error)      { panic("unused") }
func (deleteSessionStore) Get(context.Context, string) (session.Session, error) { panic("unused") }
func (deleteSessionStore) Create(context.Context, string, string) (session.Session, error) {
	panic("unused")
}
func (deleteSessionStore) Fork(context.Context, string, string) (session.Session, error) {
	panic("unused")
}
func (deleteSessionStore) Rename(context.Context, string, string) error   { panic("unused") }
func (deleteSessionStore) SetModel(context.Context, string, string) error { panic("unused") }
func (deleteSessionStore) SetCwd(context.Context, string, string) error   { panic("unused") }
func (deleteSessionStore) SetMetadata(context.Context, string, map[string]any) error {
	panic("unused")
}
func (deleteSessionStore) SetFavorite(context.Context, string, bool) error { panic("unused") }
func (deleteSessionStore) Restore(context.Context, session.Session) error  { panic("unused") }
func (deleteSessionStore) Children(context.Context, string) ([]session.Session, error) {
	panic("unused")
}
func (s deleteSessionStore) Delete(context.Context, string) error {
	return s.stores.record("session.delete")
}

type deleteTranscriptStore struct{ stores *deleteStores }

func (deleteTranscriptStore) AppendItem(context.Context, transcript.Item) error { panic("unused") }
func (deleteTranscriptStore) PutRun(context.Context, transcript.Run) error      { panic("unused") }
func (deleteTranscriptStore) DeleteRun(context.Context, string, string) error   { panic("unused") }
func (s deleteTranscriptStore) DeleteSession(context.Context, string) error {
	return s.stores.record("transcript.delete")
}

type deleteInterruptStore struct{ stores *deleteStores }

func (deleteInterruptStore) Put(context.Context, interrupts.Pending) error {
	panic("unused")
}

func (s deleteInterruptStore) List(context.Context, string) ([]interrupts.Pending, error) {
	if err := s.stores.record("interrupts.list"); err != nil {
		return nil, err
	}
	return []interrupts.Pending{{ParentRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1"}}, nil
}
func (deleteInterruptStore) Get(context.Context, string) (interrupts.Pending, bool, error) {
	panic("unused")
}
func (deleteInterruptStore) Consume(context.Context, string) (interrupts.Pending, bool, error) {
	panic("unused")
}
func (s deleteInterruptStore) Delete(context.Context, string) error {
	return s.stores.record("interrupt.delete")
}

// deleteRunsRecorder records the durable run-row drop into the cascade's shared
// operation log so a test can assert it lands inside the transaction.
type deleteRunsRecorder struct{ stores *deleteStores }

func (deleteRunsRecorder) Terminalize(context.Context, string, string) error { panic("unused") }
func (r deleteRunsRecorder) DeleteForSession(context.Context, string) error {
	return r.stores.record("runs.delete")
}

type deleteTurns struct{ operations *[]string }

func (t deleteTurns) Cancel(context.Context, RunRef) error {
	*t.operations = append(*t.operations, "turn.cancel")
	return nil
}

func (deleteTurns) Resume(context.Context, RunRef, interrupts.Resolution, []string) (Handle, error) {
	panic("unused")
}

func (deleteTurns) Rehydrate(context.Context, RehydrateSpec) (Handle, error) {
	panic("unused")
}
