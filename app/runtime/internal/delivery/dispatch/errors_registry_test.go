package dispatch

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// TestEveryDeclarableProblemTypeHasARecoveryAction is the Error Registry's own
// completeness check (§9.3): a type a method may return, with no declared action, is a
// failure a client can only guess about — which is the guessing "retryable" used to
// invite.
func TestEveryDeclarableProblemTypeHasARecoveryAction(t *testing.T) {
	t.Parallel()

	for _, problemType := range operation.ProblemTypes() {
		if !operation.IsMethodProblemType(problemType) {
			continue
		}
		if _, declared := operation.RecoveryFor(problemType); !declared {
			t.Errorf("%s is declarable and has no recovery action", problemType)
		}
	}
}

func TestMethodDeclarabilityComesFromTheRPCErrorRegistry(t *testing.T) {
	t.Parallel()

	if !operation.IsMethodProblemType(protocol.ErrRunNotFound.Error()) {
		t.Fatal("run_not_found should be method-declarable")
	}
	for _, boundaryProblem := range []string{
		protocol.ErrInvalidParams.Error(),
		protocol.ErrMethodNotFound.Error(),
		protocol.ErrIdempotencyConflict.Error(),
	} {
		if operation.IsMethodProblemType(boundaryProblem) {
			t.Fatalf("%s is a boundary failure, not a method-declarable problem", boundaryProblem)
		}
	}
}

func TestRPCErrorRegistryRejectsContradictoryMetadata(t *testing.T) {
	t.Parallel()

	valid := rpcCodeSpec{problemType: "first", code: -1}
	tests := []struct {
		name  string
		specs []rpcCodeSpec
	}{{
		name: "duplicate problem type",
		specs: []rpcCodeSpec{
			valid,
			{problemType: "first", code: -2},
		},
	}, {
		name: "duplicate numeric code",
		specs: []rpcCodeSpec{
			valid,
			{problemType: "second", code: -1},
		},
	}, {
		name:  "missing problem type",
		specs: []rpcCodeSpec{{code: -1}},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Fatal("mustRPCCodeSpecs accepted contradictory metadata")
				}
			}()
			mustRPCCodeSpecs(tt.specs)
		})
	}
}

func TestRPCErrorResolutionUsesRegistryOrder(t *testing.T) {
	t.Parallel()

	rpcErr := errorToRPC(errors.Join(
		protocol.ErrSessionNotFound,
		protocol.ErrRunNotFound,
	))
	if rpcErr.Code != codeSessionNotFound {
		t.Fatalf(
			"joined problem resolved to code %d, want first registry match %d",
			rpcErr.Code,
			codeSessionNotFound,
		)
	}
}

// TestAStructuredProblemCarriesItsPayload proves the two typed problems reach the wire
// with the fields their type requires, rather than with the payload flattened into
// prose. The presence rules published beside them say the same thing to clients; this
// is the runtime half.
func TestAStructuredProblemCarriesItsPayload(t *testing.T) {
	t.Parallel()

	conflict := problemOf(t, &operation.ActiveRunConflictError{ActiveRun: protocol.ActiveRunRef{
		RunID: "run_1", Status: protocol.RunStatusWaiting,
	}})
	if conflict.Type != protocol.ErrSessionHasActiveRun.Error() {
		t.Fatalf("type = %q, want the active-run conflict", conflict.Type)
	}
	if conflict.ActiveRun == nil || conflict.ActiveRun.RunID != "run_1" || conflict.ActiveRun.Status != protocol.RunStatusWaiting {
		t.Fatalf("activeRun = %+v, want the waiting run", conflict.ActiveRun)
	}
	if len(conflict.RequiredCapabilities) != 0 {
		t.Fatalf("requiredCapabilities = %+v on an active-run conflict", conflict.RequiredCapabilities)
	}

	// Deduplicated and ordered by (registry, name), so two refusals for the same gap
	// are the same frame instead of two transcripts of it.
	gap := problemOf(t, operation.NewCapabilityGapError(
		protocol.CapabilityRequirement{Type: protocol.RequirementInterruptType, Name: "toolResult"},
		protocol.CapabilityRequirement{Type: protocol.RequirementFeature, Name: "subagents"},
		protocol.CapabilityRequirement{Type: protocol.RequirementFeature, Name: "subagents"},
	))
	if gap.Type != protocol.ErrCapabilityNotNeg.Error() {
		t.Fatalf("type = %q, want capability_not_negotiated", gap.Type)
	}
	want := []protocol.CapabilityRequirement{
		{Type: protocol.RequirementFeature, Name: "subagents"},
		{Type: protocol.RequirementInterruptType, Name: "toolResult"},
	}
	if len(gap.RequiredCapabilities) != len(want) {
		t.Fatalf("requiredCapabilities = %+v, want %+v", gap.RequiredCapabilities, want)
	}
	for index, requirement := range want {
		if gap.RequiredCapabilities[index] != requirement {
			t.Fatalf("requiredCapabilities = %+v, want %+v", gap.RequiredCapabilities, want)
		}
	}
	if gap.ActiveRun != nil {
		t.Fatalf("activeRun = %+v on a capability gap", gap.ActiveRun)
	}
}

func problemOf(t *testing.T, err error) protocol.ProblemData {
	t.Helper()

	rpcErr := errorToRPC(err)
	if rpcErr == nil {
		t.Fatalf("%v produced no wire error", err)
	}
	var problem protocol.ProblemData
	if decodeErr := json.Unmarshal(rpcErr.Data, &problem); decodeErr != nil {
		t.Fatalf("decode problem: %v", decodeErr)
	}
	return problem
}
