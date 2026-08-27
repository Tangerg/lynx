package dispatch

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// capabilityRuntime is a Runtime that only answers discovery — enough to drive
// the gate, since that is the only thing the gate reads.
type capabilityRuntime struct {
	features map[string]bool
}

func newOperationEndpoint(t *testing.T, target any) *operation.Endpoint {
	t.Helper()
	endpoint, err := operation.New(target, operation.Config{Lifetime: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func (c *capabilityRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	advertised := make(map[string]protocol.FeatureCapability, len(c.features))
	for name, enabled := range c.features {
		published, _ := protocol.LookupFeature(name)
		advertised[name] = protocol.FeatureCapability{
			Enabled:               enabled,
			ClientOptIn:           published.ClientOptIn,
			RequiredByRunProtocol: published.RequiredByRunProtocol,
		}
	}
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: advertised}}, nil
}

func (c *capabilityRuntime) SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	return &protocol.RuntimeSubscribeResponse{}, func(func(protocol.RuntimeEvent) bool) {}, nil
}

func (c *capabilityRuntime) ListKnowledge(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
	return protocol.NewPage([]protocol.KnowledgeEntry{}), nil
}

func (c *capabilityRuntime) ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	return protocol.NewPage([]protocol.RunRef{}), nil
}

func (c *capabilityRuntime) RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	return &protocol.RollbackSessionResponse{DroppedRuns: []protocol.DroppedRun{}}, nil
}

func call(t *testing.T, features map[string]bool, method, params string) *transport.Response {
	t.Helper()
	d := New(newOperationEndpoint(t, &capabilityRuntime{features: features}))
	res := d.Dispatch(t.Context(), &transport.Request{
		ID: testID("1"), Method: method, Params: json.RawMessage(params),
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

	off := call(t, map[string]bool{"knowledge": false}, "knowledge.list", `{"workspace":{"path":"/workspace"}}`)
	if got := problemType(t, off); got != "capability_not_negotiated" {
		t.Fatalf("knowledge.list with the feature off = %q, want capability_not_negotiated", got)
	}
	on := call(t, map[string]bool{"knowledge": true}, "knowledge.list", `{"workspace":{"path":"/workspace"}}`)
	if on.Error != nil {
		t.Fatalf("knowledge.list with the feature on: %+v", on.Error)
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
		method: "runtime.subscribe", params: `{"topics":["files.changed"],"watches":[{"watchId":"w1","workspace":{"path":"/workspace"}}]}`,
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
