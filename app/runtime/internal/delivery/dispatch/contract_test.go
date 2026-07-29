package dispatch

import (
	"context"
	"encoding/json"
	"iter"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// TestContractIsTheOnlyMethodTable pins what the Registry replaced: every
// dispatchable method exists exactly once, with metadata that validates.
func TestContractIsTheOnlyMethodTable(t *testing.T) {
	t.Parallel()

	names := contract.Names()
	if len(names) == 0 {
		t.Fatal("the contract registered no methods")
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			t.Errorf("method %q is registered twice", name)
		}
		seen[name] = true
		method, ok := contract.Lookup(name)
		if !ok {
			t.Fatalf("Names() returned %q but Lookup could not find it", name)
		}
		if err := method.Meta.validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestStreamMethodsAreTheStreamingContract keeps the machine-readable streaming
// set honest: exactly the methods whose response body is their own event stream
// (TRANSPORT §6.4). A client reads this instead of hardcoding names, so a new
// stream method that forgets to register as one would go unnoticed.
func TestStreamMethodsAreTheStreamingContract(t *testing.T) {
	t.Parallel()

	want := []string{"runs.start", "runs.resume", "runs.subscribe", "runtime.subscribe"}
	got := contract.StreamMethods()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("stream methods = %v, want %v", got, want)
	}
}

// TestCapabilityRulesNameAPublishedFeature keeps the gate and the advertisement
// speaking one vocabulary.
//
// A rule requiring a feature discovery never advertises is a method NO build can
// call: the gate reads the advertised map, an unknown key reads as disabled, and
// every request comes back capability_not_negotiated. The constants make that
// unlikely; this makes it impossible.
func TestCapabilityRulesNameAPublishedFeature(t *testing.T) {
	t.Parallel()

	for _, meta := range contract.Metas() {
		for _, feature := range meta.Features() {
			if _, published := protocol.LookupFeature(feature); !published {
				t.Errorf("%s requires %q, which protocol does not publish", meta.Name, feature)
			}
		}
	}
}

// TestReplayPolicyCoversEveryMutation guards the invariant the deleted
// replay-protected list used to carry by hand: a method that opens a run replays
// by re-attaching, never by handing back a cached ack alone.
func TestReplayPolicyCoversEveryMutation(t *testing.T) {
	t.Parallel()

	for _, meta := range contract.Metas() {
		if meta.Idempotency == IdempotencyReplayRunStream && meta.Kind != KindStream {
			t.Errorf("%s: re-attach replay on a non-streaming method", meta.Name)
		}
	}
	for _, name := range []string{"runs.start", "runs.resume"} {
		method, _ := contract.Lookup(name)
		if method.Meta.Idempotency != IdempotencyReplayRunStream {
			t.Errorf("%s must replay by re-attaching to its run, got %v", name, method.Meta.Idempotency)
		}
	}
}

// capabilityRuntime is a Runtime that only answers discovery — enough to drive
// the gate, since that is the only thing the gate reads.
type capabilityRuntime struct {
	protocol.Runtime
	features map[string]bool
}

func (r *capabilityRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	advertised := make(map[string]protocol.FeatureCapability, len(r.features))
	for name, enabled := range r.features {
		published, _ := protocol.LookupFeature(name)
		advertised[name] = protocol.FeatureCapability{
			Enabled:               enabled,
			Stability:             published.Stability,
			ClientOptIn:           published.ClientOptIn,
			RequiredByRunProtocol: published.RequiredByRunProtocol,
		}
	}
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: advertised}}, nil
}

func (r *capabilityRuntime) SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	return &protocol.RuntimeSubscribeResponse{}, func(func(protocol.RuntimeEvent) bool) {}, nil
}

func (r *capabilityRuntime) ListMemory(context.Context, protocol.WorkspaceListQuery) (*protocol.Page[protocol.MemoryEntry], error) {
	return protocol.NewPage([]protocol.MemoryEntry{}), nil
}

func (r *capabilityRuntime) ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	return protocol.NewPage([]protocol.RunRef{}), nil
}

func (r *capabilityRuntime) RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	return &protocol.RollbackSessionResponse{DroppedRuns: []protocol.DroppedRun{}}, nil
}

func call(t *testing.T, features map[string]bool, method, params string) *transport.Response {
	t.Helper()
	d := New(&capabilityRuntime{features: features})
	res := d.Handle(t.Context(), &transport.Request{
		ID: transport.StringID("1"), Method: method, Params: json.RawMessage(params),
	})
	if res.Response == nil {
		t.Fatalf("%s returned no response", method)
	}
	return res.Response
}

func problemType(t *testing.T, resp *transport.Response) string {
	t.Helper()
	if resp.Error == nil {
		return ""
	}
	rpcErr, ok := resp.Error.(*transport.Error)
	if !ok {
		t.Fatalf("response error is %T, want *transport.Error", resp.Error)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return problem.Type
}

// TestCapabilityGateRefusesADisabledFeature covers the unconditional form: with
// the feature off the method is refused before the use case runs, and with it on
// the call goes through. This is the rule the 20 hand-written handler checks used
// to state — one per method, each able to drift from what discovery advertised.
func TestCapabilityGateRefusesADisabledFeature(t *testing.T) {
	t.Parallel()

	off := call(t, map[string]bool{"memory": false}, "memory.list", `{}`)
	if got := problemType(t, off); got != "capability_not_negotiated" {
		t.Fatalf("memory.list with the feature off = %q, want capability_not_negotiated", got)
	}
	on := call(t, map[string]bool{"memory": true}, "memory.list", `{}`)
	if on.Error != nil {
		t.Fatalf("memory.list with the feature on: %+v", on.Error)
	}
}

// TestCapabilityGateOnlyBitesTheGatedRequest covers the conditional form: a
// method stays usable in its default shape while one option is gated. That is why
// When exists — an unconditional rule would take the whole method away.
func TestCapabilityGateOnlyBitesTheGatedRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		params   string
		features map[string]bool
		want     string
	}{{
		name:   "subscribing without watches needs no fileWatch",
		method: "runtime.subscribe", params: `{"topics":["skills.changed"]}`,
		features: map[string]bool{"fileWatch": false},
		want:     "",
	}, {
		name:   "an empty watch list is not a watch",
		method: "runtime.subscribe", params: `{"topics":["skills.changed"],"watches":[]}`,
		features: map[string]bool{"fileWatch": false},
		want:     "",
	}, {
		name:   "registering a watch needs fileWatch",
		method: "runtime.subscribe", params: `{"topics":["files.changed"],"watches":[{"watchId":"w1"}]}`,
		features: map[string]bool{"fileWatch": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "a history rollback needs no checkpoints",
		method: "sessions.rollback", params: `{"sessionId":"ses_1"}`,
		features: map[string]bool{"checkpoints": false},
		want:     "",
	}, {
		name:   "restoring files needs checkpoints",
		method: "sessions.rollback", params: `{"sessionId":"ses_1","restoreType":"files"}`,
		features: map[string]bool{"checkpoints": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "restoring both needs checkpoints",
		method: "sessions.rollback", params: `{"sessionId":"ses_1","restoreType":"both"}`,
		features: map[string]bool{"checkpoints": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "restoring files is allowed once checkpoints are on",
		method: "sessions.rollback", params: `{"sessionId":"ses_1","restoreType":"files"}`,
		features: map[string]bool{"checkpoints": true},
		want:     "",
	}, {
		name:   "listing root runs needs no subagents",
		method: "runs.list", params: `{}`,
		features: map[string]bool{"subagents": false},
		want:     "",
	}, {
		// The whole point of the rule: the contract forbids reading an explicit true
		// as false, because the page would come back looking complete.
		name:   "asking for descendants needs subagents",
		method: "runs.list", params: `{"includeDescendants":true}`,
		features: map[string]bool{"subagents": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "server support does not replace client opt-in",
		method: "runs.list", params: `{"includeDescendants":true}`,
		features: map[string]bool{"subagents": true},
		want:     "capability_not_negotiated",
	}, {
		name:   "client opt-in and server support authorize descendants",
		method: "runs.list",
		params: `{
			"includeDescendants": true,
			"_meta": {
				"clientCapabilities": {
					"features": {"subagents": {"enabled": true}}
				}
			}
		}`,
		features: map[string]bool{"subagents": true},
		want:     "",
	}, {
		name:   "an explicit false is not a request for descendants",
		method: "runs.list", params: `{"includeDescendants":false}`,
		features: map[string]bool{"subagents": false},
		want:     "",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := call(t, tt.features, tt.method, tt.params)
			if got := problemType(t, resp); got != tt.want {
				t.Fatalf("problem type = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUnknownMethodStaysUnknown keeps the registry from becoming permissive: a
// name nobody registered is method_not_found, not a nil-handler panic.
func TestUnknownMethodStaysUnknown(t *testing.T) {
	t.Parallel()

	resp := call(t, nil, "runs.teleport", `{}`)
	if got := problemType(t, resp); got != "method_not_found" {
		t.Fatalf("problem type = %q, want method_not_found", got)
	}
}
