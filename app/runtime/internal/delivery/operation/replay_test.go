package operation

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

type claimLostOnceStore struct {
	backing *memoryIdempotencyStore
	once    sync.Once
}

func (s *claimLostOnceStore) Claim(
	ctx context.Context,
	key string,
	fingerprint string,
) (idempotency.Record, bool, error) {
	return s.backing.Claim(ctx, key, fingerprint)
}

func (s *claimLostOnceStore) Complete(ctx context.Context, record idempotency.Record) error {
	lost := false
	s.once.Do(func() {
		s.backing.mu.Lock()
		delete(s.backing.records, record.Key)
		s.backing.mu.Unlock()
		lost = true
	})
	if lost {
		return idempotency.ErrClaimLost
	}
	return s.backing.Complete(ctx, record)
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

func TestLostCompletionClaimIsReacquiredWithoutRepeatingCommand(t *testing.T) {
	service := &countingCancelService{}
	store := &claimLostOnceStore{backing: newMemoryIdempotencyStore()}
	endpoint := New(service, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "recover-lost-claim"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	const callers = 16
	errCh := make(chan error, callers)
	var callersDone sync.WaitGroup
	for range callers {
		callersDone.Add(1)
		go func() {
			defer callersDone.Done()
			response, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
				t.Context(), endpoint, "runs.cancel", request, options,
			)
			if err != nil {
				errCh <- err
				return
			}
			if response.Run.ID != "run_1" {
				errCh <- fmt.Errorf("replayed run = %q, want run_1", response.Run.ID)
			}
		}()
	}
	callersDone.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("recovered replay: %v", err)
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
