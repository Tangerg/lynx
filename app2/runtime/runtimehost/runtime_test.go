package runtimehost_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/localruntime"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runtimehost"
)

func TestRuntimePublishesUsableIdentityAndReleasesGeneration(t *testing.T) {
	dataRoot := privateDirectory(t, "data")
	spawnRoot := privateDirectory(t, "spawn")
	workspaceRoot := privateDirectory(t, "workspace")
	userHome := privateDirectory(t, "home")
	descriptorPath := filepath.Join(spawnRoot, "ready.json")
	tokenPath := filepath.Join(spawnRoot, "token")
	nonce, err := localruntime.NewNonce()
	if err != nil {
		t.Fatalf("NewNonce() error = %v", err)
	}
	host, err := runtimehost.Open(t.Context(), runtimehost.Config{
		Listen:           "127.0.0.1:0",
		DatabasePath:     filepath.Join(dataRoot, "runtime.sqlite"),
		TokenPath:        tokenPath,
		DescriptorPath:   descriptorPath,
		BootstrapNonce:   nonce,
		DefaultWorkspace: workspaceRoot,
		UserHome:         userHome,
		ServerName:       "lyra-runtime",
		ServerVersion:    "test",
		CORSOrigins:      httptransport.DefaultCORSOrigins(),
	})
	if err != nil {
		t.Fatalf("runtimehost.Open() error = %v", err)
	}
	runCtx, cancel := context.WithCancel(t.Context())
	runResult := make(chan error, 1)
	go func() { runResult <- host.Run(runCtx) }()

	descriptor := awaitDescriptor(t, descriptorPath, localruntime.Expectation{
		Root: spawnRoot, Nonce: nonce, PID: os.Getpid(), ProtocolVersion: protocol.ProtocolVersion,
	})
	token, err := localruntime.ReadToken(descriptor.TokenPath)
	if err != nil {
		t.Fatalf("ReadToken() error = %v", err)
	}
	info := getJSON(t, descriptor.BaseURL+httptransport.PathInfo, "", nil)
	ready := getJSON(t, descriptor.BaseURL+httptransport.PathReadiness, "", nil)
	discovery := getJSON(t, descriptor.BaseURL+httptransport.PathRPC, token, []byte(
		`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`,
	))
	for name, document := range map[string]map[string]any{"info": info, "ready": ready} {
		if instanceID(document) != descriptor.InstanceID {
			t.Fatalf("%s instanceId = %q, want %q", name, instanceID(document), descriptor.InstanceID)
		}
	}
	result := discovery["result"].(map[string]any)
	serverInfo := result["serverInfo"].(map[string]any)
	if serverInfo["instanceId"] != descriptor.InstanceID {
		t.Fatalf("discover instanceId = %v, want %q", serverInfo["instanceId"], descriptor.InstanceID)
	}

	cancel()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not stop")
	}
	if connection, err := net.DialTimeout("tcp", descriptorAddress(t, descriptor.BaseURL), 100*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("Runtime listener survived shutdown")
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ephemeral token remains after shutdown: %v", err)
	}
	if err := host.Close(t.Context()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestCorruptStoreNeverPublishesReadyDescriptor(t *testing.T) {
	dataRoot := privateDirectory(t, "data")
	spawnRoot := privateDirectory(t, "spawn")
	databasePath := filepath.Join(dataRoot, "runtime.sqlite")
	if err := os.WriteFile(databasePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	descriptorPath := filepath.Join(spawnRoot, "ready.json")
	_, err := runtimehost.Open(t.Context(), runtimehost.Config{
		Listen: "127.0.0.1:0", DatabasePath: databasePath,
		TokenPath: filepath.Join(spawnRoot, "token"), DescriptorPath: descriptorPath, BootstrapNonce: "nonce",
		DefaultWorkspace: "/workspace", UserHome: "/home/test", ServerName: "lyra-runtime", ServerVersion: "test",
	})
	if err == nil {
		t.Fatal("Open() accepted a corrupt store")
	}
	if _, statErr := os.Stat(descriptorPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("descriptor was published after failed open: %v", statErr)
	}
}

func TestPublicRPCIdempotencySurvivesRuntimeReplacement(t *testing.T) {
	data := privateDirectory(t, "data")
	workspace := privateDirectory(t, "workspace")
	home := privateDirectory(t, "home")
	config := runtimehost.Config{
		Listen: "127.0.0.1:0", DatabasePath: filepath.Join(data, "runtime.sqlite"),
		DefaultWorkspace: workspace, UserHome: home,
		ServerName: "lyra-runtime", ServerVersion: "test",
	}

	first := startRuntime(t, config)
	discovered := rpcCall[protocol.DiscoverResponse](t, first.baseURL, "runtime.discover", struct{}{}, "", "")
	firstNamespace := discovered.Capabilities.Limits.Idempotency.Namespace
	create := protocol.CreateSessionRequest{
		Title: "created once", Provider: "openai-compatible", Model: "test-model",
	}
	created := rpcCall[*protocol.Session](
		t, first.baseURL, "sessions.create", create, "create-once", firstNamespace,
	)
	if created.ID == "" {
		t.Fatal("sessions.create returned an empty identity")
	}
	assertRPCProblem(
		t, first.baseURL, "sessions.create", create,
		"missing-store", "", protocol.ErrIdempotencyStoreMismatch.Error(),
	)
	assertRPCProblem(
		t, first.baseURL, "sessions.list", protocol.PageQuery{},
		"query-key", firstNamespace, protocol.ErrInvalidParams.Error(),
	)
	first.stop(t)

	second := startRuntime(t, config)
	t.Cleanup(func() { second.stop(t) })
	discovered = rpcCall[protocol.DiscoverResponse](t, second.baseURL, "runtime.discover", struct{}{}, "", "")
	if discovered.ServerInfo.InstanceID == first.instanceID {
		t.Fatal("replacement Runtime reused its ephemeral instance identity")
	}
	secondNamespace := discovered.Capabilities.Limits.Idempotency.Namespace
	if secondNamespace != firstNamespace {
		t.Fatalf("idempotency namespace changed across restart: %q -> %q", firstNamespace, secondNamespace)
	}
	replayed := rpcCall[*protocol.Session](
		t, second.baseURL, "sessions.create", create, "create-once", secondNamespace,
	)
	if replayed.ID != created.ID || replayed.Revision != created.Revision || !replayed.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("replayed session = %+v, want original %+v", replayed, created)
	}
	page := rpcCall[*protocol.Page[protocol.Session]](
		t, second.baseURL, "sessions.list", protocol.PageQuery{Limit: 100}, "", "",
	)
	if len(page.Data) != 1 || page.Data[0].ID != created.ID {
		t.Fatalf("persisted sessions = %+v, want only %q", page.Data, created.ID)
	}
}

type runningRuntime struct {
	host       *runtimehost.Runtime
	baseURL    string
	instanceID string
	cancel     context.CancelFunc
	done       <-chan error
}

func startRuntime(t *testing.T, config runtimehost.Config) *runningRuntime {
	t.Helper()
	host, err := runtimehost.Open(t.Context(), config)
	if err != nil {
		t.Fatalf("runtimehost.Open() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- host.Run(ctx) }()
	baseURL := host.BaseURL()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, requestErr := http.Get(baseURL + httptransport.PathLiveness)
		if requestErr == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("Runtime at %s did not become live: %v", baseURL, requestErr)
		}
		time.Sleep(5 * time.Millisecond)
	}
	discovered := rpcCall[protocol.DiscoverResponse](t, baseURL, "runtime.discover", struct{}{}, "", "")
	return &runningRuntime{
		host: host, baseURL: baseURL, instanceID: discovered.ServerInfo.InstanceID,
		cancel: cancel, done: done,
	}
}

func (runtime *runningRuntime) stop(t *testing.T) {
	t.Helper()
	if runtime == nil || runtime.host == nil {
		return
	}
	runtime.cancel()
	select {
	case err := <-runtime.done:
		if err != nil {
			t.Fatalf("Runtime Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Runtime did not stop")
	}
	runtime.host = nil
}

type rpcFailure struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    protocol.ProblemData `json:"data"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcFailure     `json:"error"`
}

func rpcCall[Result any](
	t *testing.T,
	baseURL string,
	method string,
	params any,
	idempotencyKey string,
	idempotencyNamespace string,
) Result {
	t.Helper()
	response := rpcRequest(t, baseURL, method, params, idempotencyKey, idempotencyNamespace)
	if response.Error != nil {
		t.Fatalf("%s problem = %+v", method, response.Error.Data)
	}
	var value Result
	if err := json.Unmarshal(response.Result, &value); err != nil {
		t.Fatalf("decode %s result error = %v", method, err)
	}
	return value
}

func assertRPCProblem(
	t *testing.T,
	baseURL string,
	method string,
	params any,
	idempotencyKey string,
	idempotencyNamespace string,
	want string,
) {
	t.Helper()
	response := rpcRequest(t, baseURL, method, params, idempotencyKey, idempotencyNamespace)
	if response.Error == nil || response.Error.Data.Type != want {
		t.Fatalf("%s problem = %+v, want %q", method, response.Error, want)
	}
}

func rpcRequest(
	t *testing.T,
	baseURL string,
	method string,
	params any,
	idempotencyKey string,
	idempotencyNamespace string,
) rpcResponse {
	return rpcRequestWithMeta(
		t, baseURL, method, params, nil, idempotencyKey, idempotencyNamespace,
	)
}

func rpcRequestWithMeta(
	t *testing.T,
	baseURL string,
	method string,
	params any,
	meta *protocol.RequestMeta,
	idempotencyKey string,
	idempotencyNamespace string,
) rpcResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "acceptance", "method": method,
		"params": rpcParameters(t, params, meta),
	})
	if err != nil {
		t.Fatalf("encode %s request error = %v", method, err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+httptransport.PathRPC, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s request error = %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if idempotencyNamespace != "" {
		request.Header.Set("Idempotency-Namespace", idempotencyNamespace)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call %s error = %v", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("call %s status = %d", method, response.StatusCode)
	}
	var document rpcResponse
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatalf("decode %s response error = %v", method, err)
	}
	return document
}

func rpcParameters(t *testing.T, params any, meta *protocol.RequestMeta) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode RPC parameters error = %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil || object == nil {
		t.Fatalf("RPC parameters must encode as an object: %v", err)
	}
	if meta != nil {
		object["_meta"] = meta
	}
	return object
}

func privateDirectory(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", path, err)
	}
	return path
}

func awaitDescriptor(t *testing.T, path string, expectation localruntime.Expectation) localruntime.Descriptor {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		descriptor, err := localruntime.Consume(path, expectation)
		if err == nil {
			return descriptor
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Consume() error = %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("descriptor was not published")
		}
	}
}

func getJSON(t *testing.T, endpoint, token string, body []byte) map[string]any {
	t.Helper()
	method := http.MethodGet
	var reader *bytes.Reader
	if body != nil {
		method = http.MethodPost
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s %s status = %d", method, endpoint, response.StatusCode)
	}
	var document map[string]any
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatalf("decode %s error = %v", endpoint, err)
	}
	return document
}

func instanceID(document map[string]any) string {
	if server, ok := document["server"].(map[string]any); ok {
		return server["instanceId"].(string)
	}
	return document["instanceId"].(string)
}

func descriptorAddress(t *testing.T, baseURL string) string {
	t.Helper()
	return strings.TrimPrefix(baseURL, "http://")
}
