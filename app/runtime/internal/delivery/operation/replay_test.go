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

type competingCompletionStore struct {
	backing        *memoryIdempotencyStore
	durablePayload []byte
	durableErr     error
	once           sync.Once
}

type cancellationAwareCompletionStore struct {
	idempotency.Store
	attempts atomic.Int32
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
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

func (s *competingCompletionStore) Claim(
	ctx context.Context,
	key string,
	fingerprint string,
) (idempotency.Record, bool, error) {
	return s.backing.Claim(ctx, key, fingerprint)
}

func (s *competingCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	intercepted := false
	s.once.Do(func() {
		intercepted = true
		durable := record
		durable.Payload = s.durablePayload
		s.durableErr = s.backing.Complete(ctx, durable)
	})
	if intercepted {
		if s.durableErr != nil {
			return s.durableErr
		}
		return errors.New("completion acknowledgement was lost")
	}
	return s.backing.Complete(ctx, record)
}

func (s *cancellationAwareCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	if s.attempts.Add(1) == 1 {
		return errors.New("temporary completion failure")
	}
	s.once.Do(func() { close(s.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.release:
		return s.Store.Complete(ctx, record)
	}
}

func (s *flakyCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	if s.failures.Add(-1) >= 0 {
		return errors.New("temporary completion failure")
	}
	return s.Store.Complete(ctx, record)
}

func TestEndpointRejectsIdempotencyStoreMismatchBeforeBusinessAdmission(t *testing.T) {
	service := &countingCancelService{}
	endpoint := mustNewEndpoint(t, service, Config{IdempotencyNamespace: "idp_store_b"})
	request := protocol.CancelRunRequest{RunID: "run_1"}

	refused := endpoint.Invoke(t.Context(), "runs.cancel", request, Options{
		IdempotencyKey:       "cancel-once",
		IdempotencyNamespace: "idp_store_a",
	})
	if !errors.Is(refused.Failure, protocol.ErrIdempotencyStoreMismatch) {
		t.Fatalf("mismatch error = %v, want idempotency_store_mismatch", refused.Failure)
	}
	if got := service.calls.Load(); got != 0 {
		t.Fatalf("business calls after mismatch = %d, want 0", got)
	}

	accepted := endpoint.Invoke(t.Context(), "runs.cancel", request, Options{
		IdempotencyKey:       "cancel-once",
		IdempotencyNamespace: "idp_store_b",
	})
	if accepted.Failure != nil {
		t.Fatalf("matching namespace error = %v", accepted.Failure)
	}
	if got := service.calls.Load(); got != 1 {
		t.Fatalf("business calls after match = %d, want 1", got)
	}
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

func TestMemoryIdempotencyStoreKeepsAbandonedClaimReserved(t *testing.T) {
	store := newMemoryIdempotencyStore()
	record, claimed, err := store.Claim(t.Context(), "abandoned", "first")
	if err != nil || !claimed {
		t.Fatalf("initial claim = (%+v, %v, %v)", record, claimed, err)
	}
	store.mu.Lock()
	aged := store.records[record.Key]
	aged.expiresAt = time.Time{}
	store.records[record.Key] = aged
	store.mu.Unlock()

	got, claimed, err := store.Claim(t.Context(), record.Key, record.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("aged pending claim = (%+v, %v, %v), want reserved", got, claimed, err)
	}
	if _, _, err := store.Claim(t.Context(), record.Key, "second"); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("reuse aged pending claim = %v, want ErrKeyConflict", err)
	}
	record.Payload = []byte(`{"version":1}`)
	if err := store.Complete(t.Context(), record); err != nil {
		t.Fatalf("complete aged pending claim: %v", err)
	}
	got, claimed, err = store.Claim(t.Context(), record.Key, record.Fingerprint)
	if err != nil || claimed || string(got.Payload) != string(record.Payload) {
		t.Fatalf("completed aged claim = (%+v, %v, %v)", got, claimed, err)
	}
	store.mu.Lock()
	aged = store.records[record.Key]
	aged.expiresAt = time.Time{}
	store.records[record.Key] = aged
	store.mu.Unlock()
	got, claimed, err = store.Claim(t.Context(), record.Key, "second")
	if err != nil || !claimed || got.Fingerprint != "second" {
		t.Fatalf("replace expired result = (%+v, %v, %v)", got, claimed, err)
	}
}

func TestCompletionFailureRetriesWithoutRepeatingCommand(t *testing.T) {
	service := &countingCancelService{}
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, service, Config{IdempotencyStore: store})
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

func TestAwaitShutdownFlushesKnownCompletionBeforeStoreClosure(t *testing.T) {
	service := &countingCancelService{}
	backing := newMemoryIdempotencyStore()
	store := &flakyCompletionStore{Store: backing}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, service, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "flush-on-shutdown"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	if err := endpoint.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("AwaitShutdown: %v", err)
	}

	reopenedService := &countingCancelService{}
	reopened := mustNewEndpoint(t, reopenedService, Config{IdempotencyStore: backing})
	response, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), reopened, "runs.cancel", request, options,
	)
	if err != nil || response.Run.ID != "run_1" {
		t.Fatalf("replay after graceful shutdown = (%+v, %v)", response, err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("original CancelRun calls = %d, want 1", calls)
	}
	if calls := reopenedService.calls.Load(); calls != 0 {
		t.Fatalf("reopened CancelRun calls = %d, want 0", calls)
	}
}

func TestAwaitShutdownKeepsFailedPendingCompletionForRetry(t *testing.T) {
	service := &countingCancelService{}
	backing := newMemoryIdempotencyStore()
	store := &flakyCompletionStore{Store: backing}
	store.failures.Store(2)
	endpoint := mustNewEndpoint(t, service, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "retry-shutdown-flush"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	if err := endpoint.AwaitShutdown(t.Context()); err == nil {
		t.Fatal("first AwaitShutdown succeeded while completion persistence failed")
	}
	if err := endpoint.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry AwaitShutdown: %v", err)
	}

	reopenedService := &countingCancelService{}
	reopened := mustNewEndpoint(t, reopenedService, Config{IdempotencyStore: backing})
	if _, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), reopened, "runs.cancel", request, options,
	); err != nil {
		t.Fatalf("replay after retried shutdown flush: %v", err)
	}
	if calls := reopenedService.calls.Load(); calls != 0 {
		t.Fatalf("reopened CancelRun calls = %d, want 0", calls)
	}
}

func TestAwaitShutdownFlushHonorsOwnerCancellation(t *testing.T) {
	backing := newMemoryIdempotencyStore()
	store := &cancellationAwareCompletionStore{
		Store: backing, entered: make(chan struct{}), release: make(chan struct{}),
	}
	endpoint := mustNewEndpoint(t, &countingCancelService{}, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "cancel-shutdown-flush"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	waitCtx, cancelWait := context.WithCancel(t.Context())
	waitErr := make(chan error, 1)
	go func() { waitErr <- endpoint.AwaitShutdown(waitCtx) }()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("shutdown flush did not enter completion store")
	}
	cancelWait()
	select {
	case err := <-waitErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("AwaitShutdown cancellation = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitShutdown did not propagate owner cancellation")
	}
	close(store.release)
	if err := endpoint.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry AwaitShutdown: %v", err)
	}
}

func TestLostCompletionClaimIsReacquiredWithoutRepeatingCommand(t *testing.T) {
	service := &countingCancelService{}
	store := &claimLostOnceStore{backing: newMemoryIdempotencyStore()}
	endpoint := mustNewEndpoint(t, service, Config{IdempotencyStore: store})
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

func TestPendingCompletionReplaysDurableFirstResult(t *testing.T) {
	durableFinishedAt := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)
	durableOutcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	durablePayload, err := encodeStoredOutcome(Result{Value: &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished,
			Outcome: &durableOutcome, FinishedAt: durableFinishedAt,
		}},
	}})
	if err != nil {
		t.Fatalf("encode durable outcome: %v", err)
	}
	service := &countingCancelService{}
	store := &competingCompletionStore{
		backing:        newMemoryIdempotencyStore(),
		durablePayload: durablePayload,
	}
	endpoint := mustNewEndpoint(t, service, Config{IdempotencyStore: store})
	options := Options{IdempotencyKey: "durable-first-result"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err = Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	response, err := Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), endpoint, "runs.cancel", request, options,
	)
	if err != nil {
		t.Fatalf("replay durable first result: %v", err)
	}
	if !response.Run.FinishedAt.Equal(durableFinishedAt) {
		t.Fatalf("replayed FinishedAt = %v, want durable %v", response.Run.FinishedAt, durableFinishedAt)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestPendingCompletionRejectsKeyReuse(t *testing.T) {
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, &countingCancelService{}, Config{IdempotencyStore: store})
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
