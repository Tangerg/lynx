package dispatch

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/idempotency"
)

type blockingCancelRuntime struct {
	protocol.Runtime
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (r *blockingCancelRuntime) CancelRun(context.Context, protocol.CancelRunRequest) error {
	if r.calls.Add(1) == 1 {
		close(r.started)
		<-r.release
	}
	return nil
}

type replayRuntime struct {
	protocol.Runtime
	subscribeErr error
}

type countingCancelRuntime struct {
	protocol.Runtime
	calls atomic.Int32
}

func (r *countingCancelRuntime) CancelRun(context.Context, protocol.CancelRunRequest) error {
	r.calls.Add(1)
	return nil
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

func (r *replayRuntime) SubscribeRun(context.Context, string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	return nil, nil, r.subscribeErr
}

func TestRequestFingerprintCanonicalizesObjectOrder(t *testing.T) {
	decode := func(raw string) *transport.Request {
		t.Helper()
		message, err := transport.DecodeMessage([]byte(raw))
		if err != nil {
			t.Fatalf("decode request: %v", err)
		}
		request, ok := message.(*transport.Request)
		if !ok {
			t.Fatalf("decoded %T, want *transport.Request", message)
		}
		return request
	}

	first, err := requestFingerprint(decode(`{"jsonrpc":"2.0","id":"1","method":"sessions.create","params":{"cwd":"/tmp","title":"x"}}`))
	if err != nil {
		t.Fatalf("fingerprint first request: %v", err)
	}
	second, err := requestFingerprint(decode(`{"jsonrpc":"2.0","id":"2","method":"sessions.create","params":{"title":"x","cwd":"/tmp"}}`))
	if err != nil {
		t.Fatalf("fingerprint second request: %v", err)
	}
	if first != second {
		t.Fatalf("equivalent params produced different fingerprints: %q != %q", first, second)
	}
}

func TestReplayPreservesCompletedRunResponse(t *testing.T) {
	request, err := transport.NewCall("retry", MethodRunsStart, map[string]string{"sessionId": "ses_1"})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := transport.NewResponseResult(transport.StringID("first"), protocol.StartRunResponse{
		RunID: "run_1", SegmentID: "seg_1",
	})
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	payload, err := transport.EncodeMessage(response)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	dispatcher := &Dispatcher{api: &replayRuntime{subscribeErr: protocol.ErrRunNotFound}}

	got := dispatcher.replay(context.Background(), request, payload)
	if got.Response == nil || got.Response.Error != nil {
		t.Fatalf("replay response = %+v, want cached success", got.Response)
	}
	if got.EventStream == nil {
		t.Fatal("completed replay must return a finite stream")
	}
	if _, open := <-got.EventStream; open {
		t.Fatal("completed replay stream is not closed")
	}
}

func TestReplayClaimSerializesConcurrentMutation(t *testing.T) {
	runtime := &blockingCancelRuntime{started: make(chan struct{}), release: make(chan struct{})}
	dispatcher := New(runtime)
	ctx := transport.WithIdempotencyKey(context.Background(), "cancel-once")
	first, err := transport.NewCall("first", MethodRunsCancel, protocol.CancelRunRequest{RunID: "run_1"})
	if err != nil {
		t.Fatalf("build first request: %v", err)
	}
	second, err := transport.NewCall("second", MethodRunsCancel, protocol.CancelRunRequest{RunID: "run_1"})
	if err != nil {
		t.Fatalf("build second request: %v", err)
	}
	results := make(chan HandleResult, 2)
	go func() { results <- dispatcher.Handle(ctx, first) }()
	<-runtime.started
	go func() { results <- dispatcher.Handle(ctx, second) }()
	close(runtime.release)

	for range 2 {
		if result := <-results; result.Response == nil || result.Response.Error != nil {
			t.Fatalf("concurrent replay result = %+v", result.Response)
		}
	}
	if calls := runtime.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestCompletionFailureRetriesPersistenceWithoutRepeatingMutation(t *testing.T) {
	runtime := &countingCancelRuntime{}
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	dispatcher := New(runtime, WithIdempotencyStore(store))
	ctx := transport.WithIdempotencyKey(t.Context(), "cancel-once")
	request := func(id string, runID string) *transport.Request {
		req, err := transport.NewCall(id, MethodRunsCancel, protocol.CancelRunRequest{RunID: runID})
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		return req
	}

	first := dispatcher.Handle(ctx, request("first", "run_1"))
	var firstErr *transport.Error
	if first.Response != nil {
		firstErr, _ = errors.AsType[*transport.Error](first.Response.Error)
	}
	if first.Response == nil || firstErr == nil ||
		firstErr.Code != protocol.CodeIdempotencyInProgress {
		t.Fatalf("first response = %+v, want idempotency_in_progress", first.Response)
	}
	second := dispatcher.Handle(ctx, request("second", "run_1"))
	if second.Response == nil || second.Response.Error != nil {
		t.Fatalf("second response = %+v, want recovered success", second.Response)
	}
	third := dispatcher.Handle(ctx, request("third", "run_1"))
	if third.Response == nil || third.Response.Error != nil {
		t.Fatalf("third response = %+v, want durable replay", third.Response)
	}
	if calls := runtime.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestPendingCompletionStillRejectsKeyReuse(t *testing.T) {
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	dispatcher := New(&countingCancelRuntime{}, WithIdempotencyStore(store))
	ctx := transport.WithIdempotencyKey(t.Context(), "bound-key")
	first, _ := transport.NewCall("first", MethodRunsCancel, protocol.CancelRunRequest{RunID: "run_1"})
	dispatcher.Handle(ctx, first)
	conflict, _ := transport.NewCall("second", MethodRunsCancel, protocol.CancelRunRequest{RunID: "run_2"})
	result := dispatcher.Handle(ctx, conflict)
	var conflictErr *transport.Error
	if result.Response != nil {
		conflictErr, _ = errors.AsType[*transport.Error](result.Response.Error)
	}
	if result.Response == nil || conflictErr == nil ||
		conflictErr.Code != protocol.CodeIdempotencyConflict {
		t.Fatalf("conflict response = %+v, want idempotency_conflict", result.Response)
	}
}
