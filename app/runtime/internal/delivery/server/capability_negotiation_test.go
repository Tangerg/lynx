package server

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// withClientCapabilities builds the request context a client's `_meta` produces.
func withClientCapabilities(caps protocol.ClientCapabilities) context.Context {
	return protocol.WithRequestMeta(context.Background(), protocol.RequestMeta{ClientCapabilities: &caps})
}

// TestStartRunRefusesCapabilitiesThisBuildDoesNotHave covers the refusals §8.1
// requires instead of a silent downgrade.
func TestStartRunRefusesCapabilitiesThisBuildDoesNotHave(t *testing.T) {
	s, rt := rollbackHarness(t)
	sess, _ := rt.sess.Create(context.Background(), "s", "/w")
	request := protocol.StartRunRequest{
		SessionID: sess.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "hi"}},
	}

	tests := []struct {
		name  string
		caps  protocol.ClientCapabilities
		wants error
	}{{
		name:  "feature this vocabulary does not define",
		caps:  protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{"telepathy": {Enabled: true}}},
		wants: protocol.ErrCapabilityNotNeg,
	}, {
		name:  "interrupt type only client tools can answer",
		caps:  protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptToolResult}},
		wants: protocol.ErrCapabilityNotNeg,
	}, {
		name:  "interrupt type that is not a type",
		caps:  protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{"telepathy"}},
		wants: protocol.ErrInvalidParams,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.StartRun(withClientCapabilities(tt.caps), request)
			if !errors.Is(err, tt.wants) {
				t.Fatalf("StartRun = %v, want %v", err, tt.wants)
			}
		})
	}
}

func TestNegotiationMapsSubagentsToChildRunPolicy(t *testing.T) {
	s, _ := rollbackHarness(t)

	capabilities, err := s.negotiateCapabilities(withClientCapabilities(protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{
			protocol.FeatureSubagents: {Enabled: true},
		},
	}))
	if err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if !capabilities.ChildRuns {
		t.Fatalf("capabilities = %+v, want child runs enabled", capabilities)
	}
	wire := presentRunProtocolProfile(capabilities)
	want := []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents}
	if !slices.Equal(wire.RequiredFeatures, want) {
		t.Fatalf("wire requiredFeatures = %v, want %v", wire.RequiredFeatures, want)
	}
}

func TestCapabilitiesAdvertiseNegotiableSubagents(t *testing.T) {
	s, _ := rollbackHarness(t)

	feature, ok := s.capabilities().Features[protocol.FeatureSubagents]
	if !ok {
		t.Fatal("capabilities omit features.subagents")
	}
	if !feature.Enabled ||
		feature.Stability != protocol.StabilityStable ||
		!feature.ClientOptIn ||
		!feature.RequiredByRunProtocol {
		t.Fatalf("features.subagents = %+v, want enabled stable opt-in Run protocol feature", feature)
	}
}

func TestEveryRequiredRunFeatureHasAnApplicationPolicyMapping(t *testing.T) {
	s, _ := rollbackHarness(t)

	for _, feature := range protocol.Features() {
		if !feature.RequiredByRunProtocol {
			continue
		}
		t.Run(feature.Key, func(t *testing.T) {
			capabilities, err := s.negotiateCapabilities(withClientCapabilities(protocol.ClientCapabilities{
				Features: map[string]protocol.FeaturePreference{
					feature.Key: {Enabled: true},
				},
			}))
			if err != nil {
				t.Fatalf("negotiate required Run feature %q: %v", feature.Key, err)
			}
			if capabilities.IsEmpty() {
				t.Fatalf("required Run feature %q produced the Minimal Profile", feature.Key)
			}
		})
	}
}

// TestNegotiationDeclinedFeatureIsNotARefusal keeps the negotiation from reading
// `enabled:false` as a request: declining a capability is always honorable, and a
// client that lists every key it knows about — including ones this build lacks —
// must still be able to start a run.
func TestNegotiationDeclinedFeatureIsNotARefusal(t *testing.T) {
	s, _ := rollbackHarness(t)

	capabilities, err := s.negotiateCapabilities(withClientCapabilities(protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{
			protocol.FeatureSubagents:   {Enabled: false},
			protocol.FeatureMultimodal:  {Enabled: true},
			protocol.FeatureClientTools: {Enabled: false},
		},
		InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval, protocol.InterruptQuestion},
	}))
	if err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	// multimodal is advertised but does not reshape the run's stream, so it is not
	// part of what a later subscriber has to understand.
	if capabilities.ChildRuns {
		t.Fatal("declined subagents unexpectedly enabled child runs")
	}
	want := []execution.InterruptKind{execution.ApprovalInterrupt, execution.QuestionInterrupt}
	if len(capabilities.InterruptKinds) != len(want) {
		t.Fatalf("interruptKinds = %v, want %v", capabilities.InterruptKinds, want)
	}
}

// TestNegotiationWithoutCapabilitiesIsTheMinimalProfile pins §8.3: a client that
// declares nothing is a complete client, and the empty capability set means
// "creates no child, publishes no suspension, never parks on a human" — not a
// missing declaration to be filled in later.
func TestNegotiationWithoutCapabilitiesIsTheMinimalProfile(t *testing.T) {
	s, _ := rollbackHarness(t)

	capabilities, err := s.negotiateCapabilities(context.Background())
	if err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if !capabilities.IsEmpty() {
		t.Fatalf("capabilities = %v, want the empty Minimal Profile", capabilities)
	}
	// It reaches the wire as two empty arrays: `null` would report a known
	// contract as unknown.
	wire := presentRunProtocolProfile(capabilities)
	if wire.RequiredFeatures == nil || wire.InterruptTypes == nil {
		t.Fatalf("wire capabilities = %+v, want allocated empty sets", wire)
	}
}

// TestResumeRunRefusesACallerThatCannotFollowTheRun proves capabilities belong to
// the Run and not the request: a caller declaring less than the Run was created
// with is refused, and the interrupt stays open for a caller that can answer it.
// The alternative the contract rules out is worse than an error — the run would
// continue while publishing interrupts this caller can never resolve.
func TestResumeRunRefusesACallerThatCannotFollowTheRun(t *testing.T) {
	s, rt := rollbackHarness(t)
	rt.turns = resumeOKTurns{}
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	pending := serverPending(
		"run_1",
		sess.ID,
		"turn_parked",
		"turn_parked",
		[]transcript.Interrupt{{
			ItemID:   "item_1",
			Kind:     execution.ApprovalInterrupt,
			Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium"},
		}},
		time.Unix(1, 0).UTC(),
	)
	pending.Continuations[0].ModelSelection = mustResumeSelection(t, "openai", "gpt")
	pending.Capabilities = execution.RunCapabilities{
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt, execution.QuestionInterrupt},
	}
	if err := rt.interrupts.Open(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	answering := withClientCapabilities(protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval},
	})
	_, _, err := s.ResumeRun(answering, protocol.ResumeRunRequest{
		RunID: "run_1",
		Responses: []protocol.InterruptResponse{{
			ItemID: "item_1",
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			},
		}},
	})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("ResumeRun = %v, want capability_not_negotiated", err)
	}
	if _, found, err := rt.interrupts.Get(ctx, "run_1"); err != nil || !found {
		t.Fatalf("refused resume consumed the interrupt (found=%v err=%v)", found, err)
	}
}
