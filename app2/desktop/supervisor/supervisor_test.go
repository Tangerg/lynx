package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/sse"
)

func TestSupervisorRestartsKilledRuntimeAndReleasesEveryGeneration(t *testing.T) {
	model := questionModelServer(t)
	t.Cleanup(model.Close)
	runtimeBinary := buildRuntime(t)
	dataHome := privateDirectory(t, "data")
	workspace := privateDirectory(t, "workspace")
	userHome := privateDirectory(t, "home")
	supervisor, err := New(Config{
		RuntimeBinary:     runtimeBinary,
		DataHome:          dataHome,
		DefaultWorkspace:  workspace,
		UserHome:          userHome,
		CORSOrigins:       httptransport.DefaultCORSOrigins(),
		StartupTimeout:    5 * time.Second,
		ProbeTimeout:      time.Second,
		ShutdownTimeout:   3 * time.Second,
		RestartBackoff:    10 * time.Millisecond,
		MaxRestartBackoff: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	first, err := supervisor.Start(t.Context())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if first.Endpoint == "" || first.BearerToken == "" || first.InstanceID == "" || first.Generation != 1 {
		t.Fatalf("first connection = %+v", first.redacted())
	}
	set := func(value string) *protocol.ProviderConfigChange {
		return &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &value}
	}
	configured := runtimeRPCCall[*protocol.Provider](
		t, first, "providers.update", protocol.UpdateProviderRequest{
			Provider: "openai-compatible", BaseURL: set(model.URL), APIKey: set("test-key"),
		},
	)
	if configured.BaseURL != model.URL || configured.APIKeyMasked == "" {
		t.Fatalf("configured provider = %+v", configured)
	}
	persisted := runtimeRPCCall[*protocol.Session](
		t, first, "sessions.create", protocol.CreateSessionRequest{
			Title: "survives SIGKILL", Provider: "openai-compatible", Model: "test-model",
		},
	)
	if persisted.ID == "" {
		t.Fatal("predecessor Runtime returned an empty Session identity")
	}
	meta := protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientInfo:      &protocol.ClientInfo{Name: "supervisor-recovery", Version: "1"},
		ClientCapabilities: &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	}
	started, events := runtimeRPCStream[*protocol.StartRunResponse](
		t, first, "runs.start", protocol.StartRunRequest{
			SessionID: persisted.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "ask before crash"}},
			Provider:  "openai-compatible", Model: "test-model",
		}, meta,
	)
	if !hasSegmentOutcome(events, protocol.SegmentInterrupt) {
		t.Fatalf("predecessor Run did not wait: %+v", events)
	}
	firstRoot := supervisor.generationRoot()
	if err := first.process.Kill(); err != nil {
		t.Fatalf("kill first Runtime: %v", err)
	}

	second := awaitSuccessor(t, supervisor, first.InstanceID)
	if second.Generation != 2 || second.process.Pid == first.process.Pid {
		t.Fatalf("successor = %+v, predecessor PID = %d", second.redacted(), first.process.Pid)
	}
	if _, err := os.Stat(firstRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("predecessor generation root remains: %v", err)
	}
	recovered := runtimeRPCCall[*protocol.Session](
		t, second, "sessions.get", protocol.GetSessionRequest{SessionID: persisted.ID},
	)
	if recovered.ID != persisted.ID || recovered.Title != persisted.Title || recovered.Revision != persisted.Revision {
		t.Fatalf("successor recovered Session = %+v, want %+v", recovered, persisted)
	}
	recoveredRun := runtimeRPCCall[*protocol.RunRef](
		t, second, "runs.get", protocol.GetRunRequest{RunID: started.RunID},
	)
	if recoveredRun.Status != protocol.RunStatusWaiting {
		t.Fatalf("successor recovered Run = %+v", recoveredRun)
	}
	pending := runtimeRPCCall[*protocol.Page[protocol.PendingInterruptSet]](
		t, second, "interrupts.list", protocol.ListInterruptsRequest{
			SessionID: persisted.ID, RootRunID: started.RunID,
		},
	)
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 ||
		pending.Data[0].Interrupts[0].Type != protocol.InterruptQuestion {
		t.Fatalf("successor pending interrupts = %+v", pending.Data)
	}
	_, resumed := runtimeRPCStream[*protocol.ResumeRunResponse](
		t, second, "runs.resume", protocol.ResumeRunRequest{
			RunID: started.RunID,
			Responses: []protocol.InterruptResponse{{
				ItemID: pending.Data[0].Interrupts[0].ItemID,
				Response: protocol.InterruptResponseValue{
					Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Blue"}},
				},
			}},
		}, meta,
	)
	if !hasSegmentOutcome(resumed, protocol.SegmentCompleted) {
		t.Fatalf("successor did not complete recovered Run: %+v", resumed)
	}
	secondRoot := supervisor.generationRoot()
	secondAddress := endpointAddress(t, second.Endpoint)
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := os.Stat(secondRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successor generation root remains: %v", err)
	}
	if connection, err := net.DialTimeout("tcp", secondAddress, 100*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("successor listener remains after Close")
	}
	if err := second.process.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("successor process remains after Close")
	}
}

func runtimeRPCCall[Result any](t *testing.T, connection Connection, method string, params any) Result {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "supervisor-recovery", "method": method, "params": params,
	})
	if err != nil {
		t.Fatalf("encode %s request error = %v", method, err)
	}
	request, err := http.NewRequest(http.MethodPost, connection.Endpoint+httptransport.PathRPC, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s request error = %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+connection.BearerToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call %s error = %v", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("call %s status = %d", method, response.StatusCode)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Data protocol.ProblemData `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode %s response error = %v", method, err)
	}
	if envelope.Error != nil {
		t.Fatalf("%s problem = %+v", method, envelope.Error.Data)
	}
	var result Result
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode %s result error = %v", method, err)
	}
	return result
}

func runtimeRPCStream[Ack any](
	t *testing.T,
	connection Connection,
	method string,
	params any,
	meta protocol.RequestMeta,
) (Ack, []protocol.RunEvent) {
	t.Helper()
	encodedParams, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode %s parameters error = %v", method, err)
	}
	var parameters map[string]any
	if err := json.Unmarshal(encodedParams, &parameters); err != nil {
		t.Fatalf("materialize %s parameters error = %v", method, err)
	}
	parameters["_meta"] = meta
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "supervisor-stream-recovery", "method": method, "params": parameters,
	})
	if err != nil {
		t.Fatalf("encode %s request error = %v", method, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, connection.Endpoint+httptransport.PathRPC, bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build %s request error = %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+connection.BearerToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call %s error = %v", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("call %s status = %d", method, response.StatusCode)
	}
	reader, err := sse.NewHTTPReader(response)
	if err != nil {
		t.Fatalf("open %s SSE error = %v", method, err)
	}
	var (
		ack      Ack
		events   []protocol.RunEvent
		messageN int
	)
	for message, messageErr := range reader.Messages() {
		if messageErr != nil {
			t.Fatalf("read %s SSE error = %v", method, messageErr)
		}
		messageN++
		if messageN == 1 {
			var envelope struct {
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Data protocol.ProblemData `json:"data"`
				} `json:"error"`
			}
			if err := json.Unmarshal(message.Data, &envelope); err != nil {
				t.Fatalf("decode %s acknowledgement error = %v", method, err)
			}
			if envelope.Error != nil {
				t.Fatalf("%s acknowledgement problem = %+v", method, envelope.Error.Data)
			}
			if err := json.Unmarshal(envelope.Result, &ack); err != nil {
				t.Fatalf("decode %s acknowledgement result error = %v", method, err)
			}
			continue
		}
		var notification struct {
			Method string            `json:"method"`
			Params protocol.RunEvent `json:"params"`
		}
		if err := json.Unmarshal(message.Data, &notification); err != nil {
			t.Fatalf("decode %s event error = %v", method, err)
		}
		if notification.Method != "notifications.run.event" {
			t.Fatalf("%s notification method = %q", method, notification.Method)
		}
		events = append(events, notification.Params)
	}
	if messageN == 0 {
		t.Fatalf("%s stream returned no acknowledgement", method)
	}
	return ack, events
}

func hasSegmentOutcome(events []protocol.RunEvent, wanted protocol.SegmentOutcomeType) bool {
	for _, event := range events {
		if event.Event.Type == protocol.StreamSegmentFinished && event.Event.Outcome != nil &&
			event.Event.Outcome.Type == wanted {
			return true
		}
	}
	return false
}

func questionModelServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("model authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/models":
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprint(response, `{"object":"list","data":[{"id":"test-model","object":"model"}]}`)
		case "/chat/completions":
			var body struct {
				Messages []struct {
					Role string `json:"role"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			hasToolResult := false
			for _, message := range body.Messages {
				hasToolResult = hasToolResult || message.Role == "tool"
			}
			if hasToolResult {
				writeModelStream(response, map[string]any{"role": "assistant", "content": "recovered complete"}, "stop")
				return
			}
			writeModelStream(response, map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"index": 0, "id": "call-question", "type": "function",
					"function": map[string]any{
						"name":      "ask_user",
						"arguments": `{"fields":[{"prompt":"Pick a color","type":"choice","options":[{"label":"Blue"},{"label":"Green"}]}]}`,
					},
				}},
			}, "tool_calls")
		default:
			http.NotFound(response, request)
		}
	}))
}

func writeModelStream(response http.ResponseWriter, delta map[string]any, finishReason string) {
	response.Header().Set("Content-Type", "text/event-stream")
	chunk, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-supervisor", "object": "chat.completion.chunk", "created": 1787529600,
		"model": "test-model",
		"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": finishReason,
		}},
	})
	fmt.Fprintf(response, "data: %s\n\n", chunk)
	usage, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-supervisor", "object": "chat.completion.chunk", "created": 1787529600,
		"model": "test-model", "choices": []any{},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
	})
	fmt.Fprintf(response, "data: %s\n\ndata: [DONE]\n\n", usage)
}

func TestSupervisorFailsClosedWhenChildCannotPublish(t *testing.T) {
	supervisor, err := New(Config{
		RuntimeBinary:    "/usr/bin/false",
		DataHome:         privateDirectory(t, "data"),
		DefaultWorkspace: "/workspace", UserHome: "/home/test",
		StartupTimeout: time.Second, ProbeTimeout: 100 * time.Millisecond,
		ShutdownTimeout: time.Second, MaxStartupAttempts: 2,
		RestartBackoff: time.Millisecond, MaxRestartBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := supervisor.Start(t.Context()); err == nil {
		t.Fatal("Start() accepted a child that never published a descriptor")
	}
	if _, err := supervisor.Connection(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Connection() error = %v, want ErrUnavailable", err)
	}
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if root := supervisor.generationRoot(); root != "" {
		t.Fatalf("failed generation root remains: %q", root)
	}
}

func buildRuntime(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "lyra-runtime")
	command := exec.Command("go", "build", "-o", binary, "./cmd/lyra-runtime")
	command.Dir = "../../runtime"
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build app2 Runtime: %v\n%s", err, output)
	}
	return binary
}

func awaitSuccessor(t *testing.T, supervisor *Supervisor, predecessor string) Connection {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		connection, err := supervisor.Connection()
		if err == nil && connection.InstanceID != predecessor {
			return connection
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("successor did not become ready; last error = %v", err)
		}
	}
}

func privateDirectory(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	return path
}

func endpointAddress(t *testing.T, endpoint string) string {
	t.Helper()
	const prefix = "http://"
	if len(endpoint) <= len(prefix) || endpoint[:len(prefix)] != prefix {
		t.Fatalf("invalid endpoint %q", endpoint)
	}
	return endpoint[len(prefix):]
}
