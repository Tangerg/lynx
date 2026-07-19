package dispatch

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestIdempotencyInProgressErrorIsRetryable(t *testing.T) {
	rpcErr := errorToRPC(fmt.Errorf("%w: first execution has not completed", protocol.ErrIdempotencyInProgress))
	if rpcErr.Code != protocol.CodeIdempotencyInProgress {
		t.Fatalf("code = %d, want %d", rpcErr.Code, protocol.CodeIdempotencyInProgress)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != protocol.ErrIdempotencyInProgress.Error() || !problem.Retryable || problem.RetryAfterSeconds != 1 {
		t.Fatalf("problem = %+v", problem)
	}
}
