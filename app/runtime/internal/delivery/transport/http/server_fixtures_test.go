package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	lyrahttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const testRuntimeInstanceID = "runtime_test"

// fakeRuntime implements only the operations exercised by transport tests.
type fakeRuntime struct {
	canceledRuns   []string
	gotLastEventID string
}

func (f *fakeRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo: protocol.ServerInfo{
			Name: "lyra-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID,
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home",
		},
		Capabilities: validTestCapabilities(),
	}, nil
}

func (f *fakeRuntime) CancelRun(_ context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	f.canceledRuns = append(f.canceledRuns, in.RunID)
	outcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	finishedAt := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: in.RunID, SessionID: "ses_test", Status: protocol.RunStatusFinished,
			Outcome: &outcome, FinishedAt: finishedAt,
		}},
	}, nil
}

func validTestCapabilities() protocol.ServerCapabilities {
	return protocol.ServerCapabilities{
		RunEvents:        []protocol.StreamEventType{},
		RuntimeTopics:    []protocol.RuntimeTopic{},
		StateSnapshots:   []protocol.StateSnapshotCapability{},
		StreamingMethods: []string{},
		Features:         map[string]protocol.FeatureCapability{},
		Limits: protocol.RuntimeLimits{
			Idempotency: protocol.IdempotencyLimits{RetentionSeconds: 1, Namespace: "idp_test"},
			RunReplay: protocol.RunReplayLimits{
				Scope: protocol.ReplayScopeRuntimeInstanceRootSegment, MaxEvents: 1, MaxBytes: 1,
			},
			MCPAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: 1},
			RuntimeSubscription:      protocol.SubscriptionLimits{MaxTopics: 1, MaxWatches: 1},
		},
	}
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeRuntime) {
	t.Helper()
	api := &fakeRuntime{}
	return newTestServerFor(t, api), api
}

func newTestEndpoint(t *testing.T, target any, config operation.Config) *operation.Endpoint {
	t.Helper()
	config.Lifetime = t.Context()
	endpoint, err := operation.New(target, config)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

// newTestServerFor serves a caller-supplied Runtime through the same config, so a
// test that needs a different fake still exercises one server setup.
func newTestServerFor(t *testing.T, api any) *httptest.Server {
	t.Helper()
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Endpoint:        newTestEndpoint(t, api, operation.Config{IdempotencyNamespace: "idp_test"}),
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID},
		ProtocolVersion: testProtocolVersion,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

// decodeErrorCode reads a JSON-RPC error envelope and returns its code.
func decodeErrorCode(t *testing.T, resp *netHTTP.Response) int {
	t.Helper()
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil {
		t.Fatalf("expected an error envelope, got none")
	}
	return env.Error.Code
}

// readBody reads the response body into a string for diagnostic t.Fatalf
// messages.
func readBody(r *netHTTP.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	return buf.String()
}
