package dispatch

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// TestEveryDeclarableProblemTypeHasARecoveryAction is the Error Registry's own
// completeness check (§9.3): a type a method may return, with no declared action, is a
// failure a client can only guess about — which is the guessing "retryable" used to
// invite.
func TestEveryDeclarableProblemTypeHasARecoveryAction(t *testing.T) {
	t.Parallel()

	for _, problemType := range knownProblemTypes {
		if _, declared := RecoveryFor(problemType); !declared {
			t.Errorf("%s is declarable and has no recovery action", problemType)
		}
	}
}

// TestAStructuredProblemCarriesItsPayload proves the two typed problems reach the wire
// with the fields their type requires, rather than with the payload flattened into
// prose. The presence rules published beside them say the same thing to clients; this
// is the runtime half.
func TestAStructuredProblemCarriesItsPayload(t *testing.T) {
	t.Parallel()

	conflict := problemOf(t, &protocol.ActiveRunConflict{ActiveRun: protocol.ActiveRunRef{
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
	gap := problemOf(t, protocol.NewCapabilityGap(
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
	var _ *transport.Error = rpcErr
	return problem
}
