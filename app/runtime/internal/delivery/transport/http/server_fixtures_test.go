package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	lyrahttp "github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/http"
)

// fakeRuntime is the smallest Runtime we can pass to NewServer for
// smoke-testing the transport layer. The embedded nil protocol.Runtime
// supplies the methods the tests don't exercise; they panic if hit.
type fakeRuntime struct {
	protocol.Runtime
	canceledRuns   []string
	gotLastEventID string
}

func (f *fakeRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Protocol: protocol.SupportedProtocolRange()}, nil
}

func (f *fakeRuntime) CancelRun(_ context.Context, in protocol.CancelRunRequest) error {
	f.canceledRuns = append(f.canceledRuns, in.RunID)
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeRuntime) {
	t.Helper()
	api := &fakeRuntime{}
	srv, err := lyrahttp.NewServer(lyrahttp.Config{
		Runtime:         api,
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "lyra-test", Version: "0.0.0"},
		ProtocolVersion: testProtocolVersion,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler()), api
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
