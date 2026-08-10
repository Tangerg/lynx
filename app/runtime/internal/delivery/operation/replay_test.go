package operation

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type countingCancelService struct {
	Service
	calls atomic.Int32
}

func (s *countingCancelService) CancelRun(_ context.Context, request protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	s.calls.Add(1)
	outcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: request.RunID, SessionID: "ses_1", Status: protocol.RunStatusFinished,
			Outcome: &outcome, FinishedAt: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
		}},
	}, nil
}

type flakyCompletionStore struct {
	idempotency.Store
	failures atomic.Int32
}

func (s *flakyCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	if s.failures.Add(-1) >= 0 {
		return errors.New("temporary completion failure")
	}
	return s.Store.Complete(ctx, record)
}

func TestOperationFingerprintUsesTypedSemanticValue(t *testing.T) {
	t.Parallel()

	parameters := protocol.CreateSessionRequest{
		Title:     "session",
		Workspace: &protocol.WorkspaceRef{Path: "/workspace"},
	}
	first, err := operationFingerprint("sessions.create", parameters)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	second, err := operationFingerprint("sessions.create", parameters)
	if err != nil {
		t.Fatalf("fingerprint again: %v", err)
	}
	if first != second {
		t.Fatalf("same semantic parameters produced %q and %q", first, second)
	}
}

func TestCompletionFailureRetriesWithoutRepeatingCommand(t *testing.T) {
	service := &countingCancelService{}
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := New(service, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "cancel-once"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	for attempt := range 2 {
		response, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
			t.Context(), endpoint, "runs.cancel", request, options,
		)
		if err != nil || response.Run.ID != "run_1" {
			t.Fatalf("replay %d = (%+v, %v)", attempt, response, err)
		}
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestPendingCompletionRejectsKeyReuse(t *testing.T) {
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := New(&countingCancelService{}, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "bound-key"}

	_, _ = Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", protocol.CancelRunRequest{RunID: "run_1"}, options,
	)
	_, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", protocol.CancelRunRequest{RunID: "run_2"}, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("key reuse error = %v, want idempotency_conflict", err)
	}
}

func TestReplayRejectsUnversionedStoredOutcome(t *testing.T) {
	t.Parallel()

	method, ok := contract.lookup("runs.cancel")
	if !ok {
		t.Fatal("runs.cancel is not registered")
	}
	result := newReplayStore(newMemoryIdempotencyStore()).replay(
		t.Context(), method, []byte(`{"value":{}}`), &countingCancelService{},
	)
	if !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("replay failure = %v, want internal_error", result.Failure)
	}
}
