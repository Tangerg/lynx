package dispatch

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type blockingCancelRuntime struct {
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

func TestReplayClaimSerializesConcurrentMutation(t *testing.T) {
	runtime := &blockingCancelRuntime{started: make(chan struct{}), release: make(chan struct{})}
	router := New(operation.New(runtime, operation.Config{}))
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
