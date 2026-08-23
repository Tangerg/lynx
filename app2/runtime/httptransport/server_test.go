package httptransport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/discovery"
	"github.com/Tangerg/lynx/app2/runtime/dispatch"
	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/operation"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
	"github.com/Tangerg/sse"
	"go.opentelemetry.io/otel/trace"
)

func TestRuntimeHTTPPreservesRPCAndPublicSidecars(t *testing.T) {
	t.Parallel()

	server := newServer(t, "local-secret", []string{"http://desktop.test"}, nil)
	testServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		testServer.Close()
		_ = server.Close()
	})

	unauthorized, err := http.Post(testServer.URL+httptransport.PathRPC, "application/json", strings.NewReader(discoverRequest))
	if err != nil {
		t.Fatalf("unauthorized POST error = %v", err)
	}
	defer unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized || unauthorized.Header.Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("unauthorized response = %d, challenge=%q", unauthorized.StatusCode, unauthorized.Header.Get("WWW-Authenticate"))
	}

	request, err := http.NewRequest(http.MethodPost, testServer.URL+httptransport.PathRPC, strings.NewReader(discoverRequest))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer local-secret")
	request.Header.Set("Origin", "http://desktop.test")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("RPC request error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("RPC status = %d", response.StatusCode)
	}
	if response.Header.Get("X-Method") != "runtime.discover" || response.Header.Get("X-Server") != "lyra-runtime/dev" || response.Header.Get("Request-Id") == "" {
		t.Fatalf("RPC headers = %+v", response.Header)
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "http://desktop.test" {
		t.Fatalf("allow origin = %q", response.Header.Get("Access-Control-Allow-Origin"))
	}
	var envelope struct {
		Result protocol.DiscoverResponse `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if envelope.Result.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("protocolVersion = %q", envelope.Result.ProtocolVersion)
	}

	for _, path := range []string{httptransport.PathInfo, httptransport.PathLiveness, httptransport.PathReadiness} {
		response, err := http.Get(testServer.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, response.StatusCode)
		}
	}
}

func TestRuntimeHTTPKeepsTransportAndRPCFailuresSeparate(t *testing.T) {
	t.Parallel()

	server := newServer(t, "", nil, nil)
	testServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		testServer.Close()
		_ = server.Close()
	})

	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
		problemType string
		rpcCode     int
	}{
		{"bad media", "text/plain", discoverRequest, http.StatusUnsupportedMediaType, "unsupported_media_type", 0},
		{"bad envelope", "application/json", `{"jsonrpc":"2.0","id":7,"method":"runtime.discover"}`, http.StatusBadRequest, "invalid_request", 0},
		{"unknown operation", "application/json", `{"jsonrpc":"2.0","id":"1","method":"unknown.operation"}`, http.StatusOK, "", -32601},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := http.Post(testServer.URL+httptransport.PathRPC, test.contentType, strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("POST error = %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.status)
			}
			var body map[string]any
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode body error = %v", err)
			}
			if test.problemType != "" && body["type"] != "urn:lyra:transport:"+test.problemType {
				t.Fatalf("problem body = %+v", body)
			}
			if test.rpcCode != 0 {
				rpcError := body["error"].(map[string]any)
				if int(rpcError["code"].(float64)) != test.rpcCode {
					t.Fatalf("RPC error = %+v", rpcError)
				}
			}
		})
	}

	notification, err := http.Post(testServer.URL+httptransport.PathRPC, "application/json", strings.NewReader(`{"jsonrpc":"2.0","method":"runtime.discover"}`))
	if err != nil {
		t.Fatalf("notification POST error = %v", err)
	}
	defer notification.Body.Close()
	if notification.StatusCode != http.StatusNoContent {
		t.Fatalf("notification status = %d", notification.StatusCode)
	}

	tooLarge := bytes.Repeat([]byte{'x'}, httptransport.MaxRPCBodyBytes+1)
	oversized, err := http.Post(testServer.URL+httptransport.PathRPC, "application/json", bytes.NewReader(tooLarge))
	if err != nil {
		t.Fatalf("oversized POST error = %v", err)
	}
	defer oversized.Body.Close()
	if oversized.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", oversized.StatusCode)
	}

	wrongMethod, err := http.Get(testServer.URL + httptransport.PathRPC)
	if err != nil {
		t.Fatalf("GET RPC error = %v", err)
	}
	defer wrongMethod.Body.Close()
	if wrongMethod.StatusCode != http.StatusMethodNotAllowed || !strings.Contains(wrongMethod.Header.Get("Allow"), http.MethodPost) {
		t.Fatalf("GET RPC response = %d, Allow=%q", wrongMethod.StatusCode, wrongMethod.Header.Get("Allow"))
	}
}

func TestRuntimeHTTPCORSPreflightIsExactAndAuthFree(t *testing.T) {
	t.Parallel()

	server := newServer(t, "local-secret", []string{"http://desktop.test"}, nil)
	testServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		testServer.Close()
		_ = server.Close()
	})

	request, err := http.NewRequest(http.MethodOptions, testServer.URL+httptransport.PathRPC, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Origin", "http://desktop.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type, Last-Event-Id")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("preflight error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent || response.Header.Get("Access-Control-Allow-Origin") != "http://desktop.test" {
		t.Fatalf("preflight = %d headers=%+v", response.StatusCode, response.Header)
	}
}

func TestRuntimeHTTPStreamsAckThenReplayableFrames(t *testing.T) {
	t.Parallel()

	closed := &atomic.Bool{}
	dispatcher := streamDispatcher{stream: &sliceStream{closed: closed}}
	server, err := httptransport.New(httptransport.Config{
		Dispatcher: dispatcher,
		ServerInfo: protocol.ServerInfo{InstanceID: "ins_stream", Name: "lyra-runtime", Version: "dev"},
	})
	if err != nil {
		t.Fatalf("httptransport.New() error = %v", err)
	}
	testServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		testServer.Close()
		_ = server.Close()
	})

	response, err := http.Post(testServer.URL+httptransport.PathRPC, "application/json", strings.NewReader(discoverRequest))
	if err != nil {
		t.Fatalf("stream POST error = %v", err)
	}
	defer response.Body.Close()
	reader, err := sse.NewHTTPReader(response)
	if err != nil {
		t.Fatalf("sse.NewHTTPReader() error = %v", err)
	}
	messages := make([]sse.Message, 0, 2)
	for message, err := range reader.Messages() {
		if err != nil {
			t.Fatalf("SSE read error = %v", err)
		}
		messages = append(messages, message)
	}
	if len(messages) != 2 || messages[0].ID != "" || messages[1].ID != "evt_1" {
		t.Fatalf("SSE messages = %+v", messages)
	}
	if !bytes.Contains(messages[0].Data, []byte(`"result":{"accepted":true}`)) || !bytes.Contains(messages[1].Data, []byte(`"method":"notifications.run.event"`)) {
		t.Fatalf("SSE payloads = %q / %q", messages[0].Data, messages[1].Data)
	}
	if !closed.Load() {
		t.Fatal("stream was not closed")
	}
}

func TestRuntimeHTTPServeShutdownReleasesListener(t *testing.T) {
	server := newServer(t, "", nil, nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	address := listener.Addr().String()
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	response, err := http.Get("http://" + address + httptransport.PathLiveness)
	if err != nil {
		t.Fatalf("liveness GET error = %v", err)
	}
	response.Body.Close()
	if err := server.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	connection, err := net.Dial("tcp", address)
	if err == nil {
		connection.Close()
		t.Fatal("listener still accepts connections after shutdown")
	}
}

func TestRuntimeHTTPExtractsW3CTraceContextWithoutLoggingSecrets(t *testing.T) {
	var logs bytes.Buffer
	observedContext := make(chan trace.SpanContext, 1)
	server, err := httptransport.New(httptransport.Config{
		Dispatcher:  traceDispatcher{observed: observedContext},
		ServerInfo:  serviceInfo(),
		BearerToken: staticToken("header-secret"),
		Logger:      slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatalf("httptransport.New() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	request := httptest.NewRequest(
		http.MethodPost,
		httptransport.PathRPC+"?credential=query-secret",
		strings.NewReader(`{"jsonrpc":"2.0","id":"req_trace","method":"runtime.discover","params":{"_meta":{"clientInfo":{"name":"body-secret","version":"1"}}}}`),
	)
	request.Header.Set("Authorization", "Bearer header-secret")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	span := <-observedContext
	if got := span.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id = %q", got)
	}
	if got := span.SpanID().String(); got != "00f067aa0ba902b7" || !span.IsRemote() {
		t.Fatalf("parent span = %q, remote=%v", got, span.IsRemote())
	}
	logged := logs.String()
	for _, expected := range []string{"HTTP request completed", `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`, `"http_path":"/v2/rpc"`} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("structured log missing %q: %s", expected, logged)
		}
	}
	for _, secret := range []string{"header-secret", "query-secret", "body-secret", "Authorization"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("structured log leaked %q: %s", secret, logged)
		}
	}
}

const discoverRequest = `{"jsonrpc":"2.0","id":"req_1","method":"runtime.discover","params":{}}`

func newServer(t *testing.T, token string, origins []string, probes []httptransport.HealthProbe) *httptransport.Server {
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
	server, err := httptransport.New(httptransport.Config{
		Dispatcher:   dispatch.New(endpoint),
		ServerInfo:   serviceInfo(),
		BearerToken:  optionalStaticToken(token),
		CORSOrigins:  origins,
		HealthProbes: probes,
	})
	if err != nil {
		t.Fatalf("httptransport.New() error = %v", err)
	}
	return server
}

func serviceInfo() protocol.ServerInfo {
	return protocol.ServerInfo{InstanceID: "ins_test", Name: "lyra-runtime", Version: "dev"}
}

type streamDispatcher struct{ stream dispatch.Stream }

type staticToken string

func (token staticToken) Token(context.Context) (string, error) {
	return string(token), nil
}

func optionalStaticToken(value string) httptransport.TokenSource {
	if value == "" {
		return nil
	}
	return staticToken(value)
}

func (dispatcher streamDispatcher) Dispatch(_ context.Context, message rpcwire.Message, _ dispatch.Metadata) dispatch.Result {
	request := message.(*rpcwire.Request)
	response, _ := rpcwire.NewResult(request.ID, map[string]bool{"accepted": true})
	return dispatch.Result{Response: response, Stream: dispatcher.stream}
}

type traceDispatcher struct{ observed chan<- trace.SpanContext }

func (dispatcher traceDispatcher) Dispatch(ctx context.Context, message rpcwire.Message, _ dispatch.Metadata) dispatch.Result {
	dispatcher.observed <- trace.SpanContextFromContext(ctx)
	request := message.(*rpcwire.Request)
	response, _ := rpcwire.NewResult(request.ID, map[string]bool{"accepted": true})
	return dispatch.Result{Response: response}
}

type sliceStream struct {
	index  int
	closed *atomic.Bool
}

func (stream *sliceStream) Next(context.Context) (dispatch.StreamFrame, error) {
	if stream.index > 0 {
		return dispatch.StreamFrame{}, io.EOF
	}
	stream.index++
	notification, err := rpcwire.NewNotification("notifications.run.event", map[string]string{"eventId": "evt_1"})
	if err != nil {
		return dispatch.StreamFrame{}, err
	}
	return dispatch.StreamFrame{EventID: "evt_1", Message: notification}, nil
}

func (stream *sliceStream) Close() error {
	if !stream.closed.CompareAndSwap(false, true) {
		return errors.New("stream closed twice")
	}
	return nil
}
