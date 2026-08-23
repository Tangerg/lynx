package dispatch_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/dispatch"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
)

func TestRouterDispatchesTypedRequestAndExtractsMetadata(t *testing.T) {
	t.Parallel()

	router := newRouter(t)
	message, err := rpcwire.Decode([]byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":"req_1",
		"method":"runtime.discover",
		"params":{"_meta":{"protocolVersion":%q}}
	}`, protocol.ProtocolVersion)))
	if err != nil {
		t.Fatalf("rpcwire.Decode() error = %v", err)
	}
	result := router.Dispatch(t.Context(), message, dispatch.Metadata{})
	if result.Response == nil || result.Response.Error != nil {
		t.Fatalf("Dispatch() response = %#v", result.Response)
	}
	encoded, err := rpcwire.Encode(result.Response)
	if err != nil {
		t.Fatalf("rpcwire.Encode() error = %v", err)
	}
	var response struct {
		Result protocol.DiscoverResponse `json:"result"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Result.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("protocolVersion = %q", response.Result.ProtocolVersion)
	}
}

func TestRouterUsesCurrentLyraProblemCodes(t *testing.T) {
	t.Parallel()

	router := newRouter(t)
	tests := []struct {
		name     string
		body     string
		code     int64
		typeName string
	}{
		{
			name:     "method not found",
			body:     `{"jsonrpc":"2.0","id":"1","method":"missing.operation","params":{}}`,
			code:     -32601,
			typeName: protocol.ErrMethodNotFound.Error(),
		},
		{
			name:     "invalid params",
			body:     `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{"unexpected":true}}`,
			code:     -32602,
			typeName: protocol.ErrInvalidParams.Error(),
		},
		{
			name:     "invalid protocol version",
			body:     `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{"_meta":{"protocolVersion":"future"}}}`,
			code:     -32016,
			typeName: protocol.ErrInvalidProtocolVersion.Error(),
		},
		{
			name:     "null client capabilities",
			body:     `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{"_meta":{"clientCapabilities":null}}}`,
			code:     -32602,
			typeName: protocol.ErrInvalidParams.Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := rpcwire.Decode([]byte(test.body))
			if err != nil {
				t.Fatalf("rpcwire.Decode() error = %v", err)
			}
			response := router.Dispatch(t.Context(), message, dispatch.Metadata{}).Response
			if response == nil || response.Error == nil {
				t.Fatalf("response = %#v", response)
			}
			encoded, err := rpcwire.Encode(response)
			if err != nil {
				t.Fatalf("rpcwire.Encode() error = %v", err)
			}
			var envelope struct {
				Error struct {
					Code    int64  `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(encoded, &envelope); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if envelope.Error.Code != test.code || envelope.Error.Message != test.typeName {
				t.Fatalf("RPC error = %+v, want code=%d message=%q", envelope.Error, test.code, test.typeName)
			}
		})
	}
}

func TestRouterDoesNotReplyToNotifications(t *testing.T) {
	t.Parallel()

	message, err := rpcwire.Decode([]byte(`{"jsonrpc":"2.0","method":"runtime.discover","params":{}}`))
	if err != nil {
		t.Fatalf("rpcwire.Decode() error = %v", err)
	}
	result := newRouter(t).Dispatch(t.Context(), message, dispatch.Metadata{})
	if result.Response != nil {
		t.Fatalf("notification response = %#v", result.Response)
	}
}

func newRouter(t *testing.T) *dispatch.Router {
	t.Helper()
	service, err := discovery.New(discovery.Config{
		ServerInfo: protocol.ServerInfo{
			InstanceID: "ins_test", Name: "lyra-runtime", Version: "dev",
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home/test",
		},
		IdempotencyNamespace: "idp_test",
	})
	if err != nil {
		t.Fatalf("discovery.New() error = %v", err)
	}
	endpoint, err := operation.New(service, t.Context())
	if err != nil {
		t.Fatalf("operation.New() error = %v", err)
	}
	return dispatch.New(endpoint)
}
