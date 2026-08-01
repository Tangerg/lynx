package dispatch

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// The symbol says the first execution is still running; the backoff says how
// long to wait. A separate retryable flag said neither and was redundant with
// both — the wire omits false, so a client could never tell "not retryable" from
// "unclassified" anyway.
func TestIdempotencyInProgressErrorCarriesItsBackoff(t *testing.T) {
	rpcErr := errorToRPC(fmt.Errorf("%w: first execution has not completed", protocol.ErrIdempotencyInProgress))
	if rpcErr.Code != protocol.CodeIdempotencyInProgress {
		t.Fatalf("code = %d, want %d", rpcErr.Code, protocol.CodeIdempotencyInProgress)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != protocol.ErrIdempotencyInProgress.Error() || problem.RetryAfterSeconds != 1 {
		t.Fatalf("problem = %+v", problem)
	}
}

func TestMalformedStructuredProblemFailsClosedAsInternalError(t *testing.T) {
	t.Parallel()

	// capability_not_negotiated without its non-empty requirements is not a legal
	// frame. The error boundary must not publish a payload whose recovery contract
	// it cannot satisfy.
	rpcErr := problemError(
		protocol.ErrCapabilityNotNeg,
		"missing structured requirements",
	)
	if rpcErr.Code != protocol.CodeInternalError || rpcErr.Message != protocol.ProblemInternalError {
		t.Fatalf("fallback error = %+v, want internal_error", rpcErr)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode fallback problem: %v", err)
	}
	if problem.Type != protocol.ProblemInternalError {
		t.Fatalf("fallback problem type = %q, want %q", problem.Type, protocol.ProblemInternalError)
	}
}
