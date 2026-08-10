package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
)

type blockingCancelRuntime struct {
	protocol.Runtime
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (r *blockingCancelRuntime) CancelRun(_ context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	if r.calls.Add(1) == 1 {
		close(r.started)
		<-r.release
	}
	return canceledRootResponse(in.RunID), nil
}

type replayRuntime struct {
	protocol.Runtime
	subscribeErr error
}

type countingCancelRuntime struct {
	protocol.Runtime
	calls atomic.Int32
}

func (r *countingCancelRuntime) CancelRun(_ context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	r.calls.Add(1)
	return canceledRootResponse(in.RunID), nil
}

func canceledRootResponse(runID string) *protocol.CancelRunResponse {
	outcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	finishedAt := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: runID, SessionID: "ses_1", Status: protocol.RunStatusFinished,
			Outcome: &outcome, FinishedAt: finishedAt,
		}},
	}
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

func (r *replayRuntime) SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
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

	first, err := requestFingerprint(decode(`{"jsonrpc":"2.0","id":"1","method":"sessions.create","params":{"workspace":{"path":"/tmp"},"title":"x"}}`))
	if err != nil {
		t.Fatalf("fingerprint first request: %v", err)
	}
	second, err := requestFingerprint(decode(`{"jsonrpc":"2.0","id":"2","method":"sessions.create","params":{"title":"x","workspace":{"path":"/tmp"}}}`))
	if err != nil {
		t.Fatalf("fingerprint second request: %v", err)
	}
	if first != second {
		t.Fatalf("equivalent params produced different fingerprints: %q != %q", first, second)
	}
}

func TestReplayPreservesCompletedRunResponse(t *testing.T) {
	request, err := transport.NewCall("retry", "runs.start", map[string]string{"sessionId": "ses_1"})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := transport.NewResponseResult(transport.StringID("first"), protocol.StartRunResponse{
		RunID: "run_1", SegmentID: "seg_1", UserItemID: "item_1",
	})
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	payload, err := transport.EncodeMessage(response)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	router := &Router{api: &replayRuntime{subscribeErr: protocol.ErrRunNotFound}}

	got := router.replay(context.Background(), request, payload)
	if got.Response == nil || got.Response.Error != nil {
		t.Fatalf("replay response = %+v, want cached success", got.Response)
	}
	if got.EventStream == nil {
		t.Fatal("completed replay must return a finite stream")
	}
	for range got.EventStream { // a finished replay yields no frames
		t.Fatal("completed replay stream is not empty")
	}
}

func TestReplayClaimSerializesConcurrentMutation(t *testing.T) {
	runtime := &blockingCancelRuntime{started: make(chan struct{}), release: make(chan struct{})}
	router := New(runtime, Config{})
	ctx := transport.WithIdempotencyKey(context.Background(), "cancel-once")
	first, err := transport.NewCall("first", "runs.cancel", protocol.CancelRunRequest{RunID: "run_1"})
	if err != nil {
		t.Fatalf("build first request: %v", err)
	}
	second, err := transport.NewCall("second", "runs.cancel", protocol.CancelRunRequest{RunID: "run_1"})
	if err != nil {
		t.Fatalf("build second request: %v", err)
	}
	results := make(chan Result, 2)
	go func() { results <- router.Dispatch(ctx, first) }()
	<-runtime.started
	go func() { results <- router.Dispatch(ctx, second) }()
	close(runtime.release)

	var replayedResults []string
	for range 2 {
		if result := <-results; result.Response == nil || result.Response.Error != nil {
			t.Fatalf("concurrent replay result = %+v", result.Response)
		} else {
			replayedResults = append(replayedResults, string(result.Response.Result))
		}
	}
	if replayedResults[0] != replayedResults[1] {
		t.Fatalf("replayed result changed:\nfirst:  %s\nsecond: %s", replayedResults[0], replayedResults[1])
	}
	var canceled protocol.CancelRunResponse
	if err := json.Unmarshal([]byte(replayedResults[0]), &canceled); err != nil {
		t.Fatalf("decode typed cancel result: %v", err)
	}
	if canceled.Type != protocol.CancelRunRoot || canceled.Run.ID != "run_1" {
		t.Fatalf("typed cancel result = %+v, want root run_1", canceled)
	}
	if calls := runtime.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestCompletionFailureRetriesPersistenceWithoutRepeatingMutation(t *testing.T) {
	runtime := &countingCancelRuntime{}
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	router := New(runtime, Config{IdempotencyStore: store})
	ctx := transport.WithIdempotencyKey(t.Context(), "cancel-once")
	request := func(id string, runID string) *transport.Request {
		req, err := transport.NewCall(id, "runs.cancel", protocol.CancelRunRequest{RunID: runID})
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		return req
	}

	first := router.Dispatch(ctx, request("first", "run_1"))
	var firstErr *transport.Error
	if first.Response != nil {
		firstErr, _ = errors.AsType[*transport.Error](first.Response.Error)
	}
	if first.Response == nil || firstErr == nil ||
		firstErr.Code != protocol.CodeIdempotencyInProgress {
		t.Fatalf("first response = %+v, want idempotency_in_progress", first.Response)
	}
	second := router.Dispatch(ctx, request("second", "run_1"))
	if second.Response == nil || second.Response.Error != nil {
		t.Fatalf("second response = %+v, want recovered success", second.Response)
	}
	third := router.Dispatch(ctx, request("third", "run_1"))
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
	router := New(&countingCancelRuntime{}, Config{IdempotencyStore: store})
	ctx := transport.WithIdempotencyKey(t.Context(), "bound-key")
	first, _ := transport.NewCall("first", "runs.cancel", protocol.CancelRunRequest{RunID: "run_1"})
	router.Dispatch(ctx, first)
	conflict, _ := transport.NewCall("second", "runs.cancel", protocol.CancelRunRequest{RunID: "run_2"})
	result := router.Dispatch(ctx, conflict)
	var conflictErr *transport.Error
	if result.Response != nil {
		conflictErr, _ = errors.AsType[*transport.Error](result.Response.Error)
	}
	if result.Response == nil || conflictErr == nil ||
		conflictErr.Code != protocol.CodeIdempotencyConflict {
		t.Fatalf("conflict response = %+v, want idempotency_conflict", result.Response)
	}
}
