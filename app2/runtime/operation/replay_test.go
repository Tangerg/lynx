package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/idempotency"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestCommandIdempotencyReplaysFirstOutcomeAndRejectsKeyReuse(t *testing.T) {
	service := &replaySessionService{}
	endpoint := replayEndpoint(t, service)
	options := operation.Options{
		IdempotencyKey: "create-once", IdempotencyNamespace: "idp_test",
	}
	request := protocol.CreateSessionRequest{Title: "first"}

	first, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if err != nil {
		t.Fatalf("first sessions.create error = %v", err)
	}
	second, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if err != nil {
		t.Fatalf("replayed sessions.create error = %v", err)
	}
	if first.ID != second.ID || service.createCount() != 1 {
		t.Fatalf("replay = %q/%q, business calls = %d", first.ID, second.ID, service.createCount())
	}

	conflict := request
	conflict.Title = "different"
	_, err = operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", conflict, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyConflict) || service.createCount() != 1 {
		t.Fatalf("key reuse error = %v, business calls = %d", err, service.createCount())
	}
}

func TestIdempotencyNamespaceAndCapabilitiesFenceBusinessAdmission(t *testing.T) {
	service := &replaySessionService{}
	endpoint := replayEndpoint(t, service)
	request := protocol.CreateSessionRequest{Title: "bounded"}

	_, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request,
		operation.Options{IdempotencyKey: "wrong-store", IdempotencyNamespace: "idp_other"},
	)
	if !errors.Is(err, protocol.ErrIdempotencyStoreMismatch) || service.createCount() != 0 {
		t.Fatalf("store mismatch error = %v, business calls = %d", err, service.createCount())
	}

	first := operation.Options{
		IdempotencyKey: "capability-bound", IdempotencyNamespace: "idp_test",
		RequestMeta: protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		}},
	}
	if _, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, first,
	); err != nil {
		t.Fatalf("capability-bound first call error = %v", err)
	}
	changed := first
	changed.RequestMeta.ClientCapabilities = &protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval},
	}
	_, err = operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, changed,
	)
	if !errors.Is(err, protocol.ErrIdempotencyConflict) || service.createCount() != 1 {
		t.Fatalf("capability key reuse error = %v, business calls = %d", err, service.createCount())
	}
}

func TestCommandIdempotencyReplaysSafeProblem(t *testing.T) {
	service := &replaySessionService{failure: protocol.ErrWorkspaceUnavailable}
	endpoint := replayEndpoint(t, service)
	options := operation.Options{
		IdempotencyKey: "failed-once", IdempotencyNamespace: "idp_test",
	}
	request := protocol.CreateSessionRequest{Title: "failure"}

	for attempt := 0; attempt < 2; attempt++ {
		_, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
			t.Context(), endpoint, "sessions.create", request, options,
		)
		if !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
			t.Fatalf("attempt %d error = %v", attempt+1, err)
		}
	}
	if service.createCount() != 1 {
		t.Fatalf("failed business calls = %d, want 1", service.createCount())
	}
}

func TestRunOpeningReplayReattachesWithoutStartingAnotherRun(t *testing.T) {
	service := &replayRunService{}
	endpoint := replayEndpoint(t, service)
	options := operation.Options{
		IdempotencyKey: "run-once", IdempotencyNamespace: "idp_test",
	}
	request := protocol.StartRunRequest{
		SessionID: "ses_test",
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "hello"}},
		Provider:  "openai-compatible", Model: "test-model",
	}

	first, events, err := operation.CallStream[
		protocol.StartRunRequest,
		*protocol.StartRunResponse,
		protocol.RunEvent,
	](t.Context(), endpoint, "runs.start", request, options)
	if err != nil {
		t.Fatalf("first runs.start error = %v", err)
	}
	for _, eventErr := range events {
		if eventErr != nil {
			t.Fatalf("first stream error = %v", eventErr)
		}
	}

	second, replayed, err := operation.CallStream[
		protocol.StartRunRequest,
		*protocol.StartRunResponse,
		protocol.RunEvent,
	](t.Context(), endpoint, "runs.start", request, options)
	if err != nil {
		t.Fatalf("replayed runs.start error = %v", err)
	}
	for _, eventErr := range replayed {
		if eventErr != nil {
			t.Fatalf("replayed stream error = %v", eventErr)
		}
	}
	starts, subscriptions := service.counts()
	if first.RunID != second.RunID || starts != 1 || subscriptions != 1 {
		t.Fatalf("run replay = %q/%q, starts=%d subscriptions=%d", first.RunID, second.RunID, starts, subscriptions)
	}
}

func TestCompletionFailureRetriesWithoutRepeatingBusinessMutation(t *testing.T) {
	service := &replaySessionService{}
	backing := newReplayMemoryStore(t)
	store := &flakyReplayStore{Store: backing, failures: 1}
	endpoint := replayEndpointWithStore(t, service, store)
	options := operation.Options{
		IdempotencyKey: "retry-receipt", IdempotencyNamespace: "idp_test",
	}
	request := protocol.CreateSessionRequest{Title: "one mutation"}

	_, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
			t.Context(), endpoint, "sessions.create", request, options,
		); err != nil {
			t.Fatalf("retry %d error = %v", attempt+1, err)
		}
	}
	if service.createCount() != 1 {
		t.Fatalf("business calls = %d, want 1", service.createCount())
	}
}

func TestLostCompletionAcknowledgementReplaysDurableFirstOutcome(t *testing.T) {
	service := &replaySessionService{}
	backing := newReplayMemoryStore(t)
	durable := replaySession("ses_durable", "durable first", 1)
	store := &lostAcknowledgementStore{
		Store: backing, durablePayload: storedSessionOutcome(t, durable),
	}
	endpoint := replayEndpointWithStore(t, service, store)
	options := operation.Options{
		IdempotencyKey: "lost-ack", IdempotencyNamespace: "idp_test",
	}
	request := protocol.CreateSessionRequest{Title: "local outcome"}

	_, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	replayed, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if err != nil {
		t.Fatalf("replay error = %v", err)
	}
	if replayed.ID != durable.ID || service.createCount() != 1 {
		t.Fatalf("replay ID = %q, business calls = %d", replayed.ID, service.createCount())
	}
}

func TestLostClaimIsReacquiredWithoutRepeatingBusinessMutation(t *testing.T) {
	service := &replaySessionService{}
	store := &claimLostReplayStore{Store: newReplayMemoryStore(t)}
	endpoint := replayEndpointWithStore(t, service, store)
	options := operation.Options{
		IdempotencyKey: "lost-claim", IdempotencyNamespace: "idp_test",
	}
	request := protocol.CreateSessionRequest{Title: "known outcome"}

	first, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}
	replayed, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	)
	if err != nil || replayed.ID != first.ID || service.createCount() != 1 {
		t.Fatalf("replay = %+v, error = %v, business calls = %d", replayed, err, service.createCount())
	}
}

func TestShutdownFlushesKnownOutcomeBeforeStoreClosure(t *testing.T) {
	service := &replaySessionService{}
	backing := newReplayMemoryStore(t)
	endpoint := replayEndpointWithStore(
		t, service, &flakyReplayStore{Store: backing, failures: 1},
	)
	options := operation.Options{
		IdempotencyKey: "flush-on-close", IdempotencyNamespace: "idp_test",
	}
	request := protocol.CreateSessionRequest{Title: "flush"}
	if _, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), endpoint, "sessions.create", request, options,
	); !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	if err := endpoint.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("AwaitShutdown() error = %v", err)
	}

	reopenedService := &replaySessionService{}
	reopened := replayEndpointWithStore(t, reopenedService, backing)
	if _, err := operation.Call[protocol.CreateSessionRequest, *protocol.Session](
		t.Context(), reopened, "sessions.create", request, options,
	); err != nil {
		t.Fatalf("replay after shutdown error = %v", err)
	}
	if reopenedService.createCount() != 0 || service.createCount() != 1 {
		t.Fatalf("business calls before/after reopen = %d/%d", service.createCount(), reopenedService.createCount())
	}
}

type replaySessionService struct {
	mu      sync.Mutex
	creates int
	failure error
}

func (service *replaySessionService) CreateSession(
	_ context.Context,
	request protocol.CreateSessionRequest,
) (*protocol.Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.creates++
	if service.failure != nil {
		return nil, service.failure
	}
	return replaySession(fmt.Sprintf("ses_%d", service.creates), request.Title, service.creates), nil
}

func (service *replaySessionService) createCount() int {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.creates
}

type replayRunService struct {
	mu            sync.Mutex
	starts        int
	subscriptions int
}

func (service *replayRunService) StartRun(
	context.Context,
	protocol.StartRunRequest,
) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
	service.mu.Lock()
	service.starts++
	service.mu.Unlock()
	return &protocol.StartRunResponse{
		RunID: "run_test", SegmentID: "seg_test", UserItemID: "itm_test",
	}, emptyRunEvents, nil
}

func (service *replayRunService) SubscribeRun(
	context.Context,
	protocol.SubscribeRunRequest,
) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
	service.mu.Lock()
	service.subscriptions++
	service.mu.Unlock()
	return &protocol.SubscribeRunResponse{
		RunID: "run_test", SegmentID: "seg_test",
	}, emptyRunEvents, nil
}

func (service *replayRunService) counts() (int, int) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.starts, service.subscriptions
}

var emptyRunEvents iter.Seq[protocol.RunEvent] = func(func(protocol.RunEvent) bool) {}

func replayEndpoint(t *testing.T, target any) *operation.Endpoint {
	t.Helper()
	return replayEndpointWithStore(t, target, newReplayMemoryStore(t))
}

func replayEndpointWithStore(t *testing.T, target any, store idempotency.Store) *operation.Endpoint {
	t.Helper()
	endpoint, err := operation.New(target, operation.Config{
		Lifetime: t.Context(), IdempotencyStore: store,
		IdempotencyNamespace: "idp_test",
	})
	if err != nil {
		t.Fatalf("operation.New() error = %v", err)
	}
	return endpoint
}

func newReplayMemoryStore(t *testing.T) *idempotency.MemoryStore {
	t.Helper()
	store, err := idempotency.NewMemoryStore(24 * time.Hour)
	if err != nil {
		t.Fatalf("idempotency.NewMemoryStore() error = %v", err)
	}
	return store
}

func replaySession(id string, title string, offset int) *protocol.Session {
	now := time.Date(2026, time.August, 24, 0, 0, offset, 0, time.UTC)
	return &protocol.Session{
		ID: id, Title: title, Status: protocol.SessionStatusIdle, Model: "test-model",
		Workspace: protocol.WorkspaceInfo{
			Ref:         protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable,
		},
		CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
}

func storedSessionOutcome(t *testing.T, session *protocol.Session) []byte {
	t.Helper()
	payload, err := json.Marshal(struct {
		Version int               `json:"version"`
		Value   *protocol.Session `json:"value"`
	}{Version: 1, Value: session})
	if err != nil {
		t.Fatalf("encode stored Session outcome error = %v", err)
	}
	return payload
}

type flakyReplayStore struct {
	idempotency.Store
	mu       sync.Mutex
	failures int
}

func (store *flakyReplayStore) Complete(
	ctx context.Context,
	record idempotency.Record,
) (idempotency.Record, error) {
	store.mu.Lock()
	if store.failures > 0 {
		store.failures--
		store.mu.Unlock()
		return idempotency.Record{}, errors.New("temporary receipt failure")
	}
	store.mu.Unlock()
	return store.Store.Complete(ctx, record)
}

type lostAcknowledgementStore struct {
	idempotency.Store
	once           sync.Once
	durablePayload []byte
}

func (store *lostAcknowledgementStore) Complete(
	ctx context.Context,
	record idempotency.Record,
) (idempotency.Record, error) {
	lost := false
	store.once.Do(func() {
		lost = true
		record.Payload = store.durablePayload
		_, _ = store.Store.Complete(ctx, record)
	})
	if lost {
		return idempotency.Record{}, errors.New("completion acknowledgement was lost")
	}
	return store.Store.Complete(ctx, record)
}

type claimLostReplayStore struct {
	idempotency.Store
	once sync.Once
}

func (store *claimLostReplayStore) Complete(
	ctx context.Context,
	record idempotency.Record,
) (idempotency.Record, error) {
	lost := false
	store.once.Do(func() {
		lost = true
		replacement, err := idempotency.NewMemoryStore(24 * time.Hour)
		if err != nil {
			panic(err)
		}
		store.Store = replacement
	})
	if lost {
		return idempotency.Record{}, idempotency.ErrClaimLost
	}
	return store.Store.Complete(ctx, record)
}
