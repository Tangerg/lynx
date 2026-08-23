package runtimehost_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runtimehost"
	"github.com/Tangerg/sse"
)

func TestPublicRunLifecycleNormalQuestionApprovalAndDelegation(t *testing.T) {
	model := newScenarioModel(t)
	t.Cleanup(model.Close)
	data := privateDirectory(t, "data")
	workspace := privateDirectory(t, "workspace")
	home := privateDirectory(t, "home")
	runtime := startRuntime(t, runtimehost.Config{
		Listen: "127.0.0.1:0", DatabasePath: filepath.Join(data, "runtime.sqlite"),
		DefaultWorkspace: workspace, UserHome: home,
		ServerName: "lyra-runtime", ServerVersion: "acceptance",
	})
	t.Cleanup(func() { runtime.stop(t) })

	discovered := rpcCall[protocol.DiscoverResponse](
		t, runtime.baseURL, "runtime.discover", struct{}{}, "", "",
	)
	namespace := discovered.Capabilities.Limits.Idempotency.Namespace
	set := func(value string) *protocol.ProviderConfigChange {
		return &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &value}
	}
	configured := rpcCall[*protocol.Provider](
		t, runtime.baseURL, "providers.update", protocol.UpdateProviderRequest{
			Provider: "openai-compatible", BaseURL: set(model.URL), APIKey: set("test-key"),
		}, "configure-provider", namespace,
	)
	if configured.APIKeyMasked == "" || configured.BaseURL != model.URL {
		t.Fatalf("configured provider = %+v", configured)
	}
	probe := rpcCall[*protocol.ProviderTestResult](
		t, runtime.baseURL, "providers.test",
		protocol.TestProviderRequest{Provider: "openai-compatible"}, "", "",
	)
	if !probe.OK || probe.Error != nil {
		t.Fatalf("provider probe = %+v", probe)
	}

	meta := protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientInfo:      &protocol.ClientInfo{Name: "runtime-acceptance", Version: "1"},
		ClientCapabilities: &protocol.ClientCapabilities{
			Features: map[string]protocol.FeaturePreference{
				protocol.FeatureSubagents: {Enabled: true},
			},
			InterruptTypes: []protocol.InterruptType{
				protocol.InterruptApproval, protocol.InterruptQuestion,
			},
		},
	}

	t.Run("normal", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "normal")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_NORMAL"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "normal-run", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, events)
		assertFinishedRun(t, runtime.baseURL, ack.RunID, meta)
		items := rpcCallWithMeta[*protocol.ListItemsResponse](
			t, runtime.baseURL, "items.list", protocol.ListItemsRequest{
				Scope:     protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID},
				PageQuery: protocol.PageQuery{Limit: 100},
			}, meta,
		)
		if !containsFinalText(items.Data, "normal complete") {
			t.Fatalf("normal transcript omitted final answer: %+v", items.Data)
		}
	})

	t.Run("question", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "question")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_QUESTION"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "question-run", namespace,
		)
		assertInterruptedRunStream(t, ack.RunID, events)
		pending := rpcCallWithMeta[*protocol.Page[protocol.PendingInterruptSet]](
			t, runtime.baseURL, "interrupts.list",
			protocol.ListInterruptsRequest{SessionID: session.ID, RootRunID: ack.RunID}, meta,
		)
		if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 ||
			pending.Data[0].Interrupts[0].Type != protocol.InterruptQuestion {
			t.Fatalf("pending question = %+v", pending.Data)
		}
		interrupt := pending.Data[0].Interrupts[0]
		resumed, resumedEvents := rpcRunStream[*protocol.ResumeRunResponse](
			t, runtime.baseURL, "runs.resume", protocol.ResumeRunRequest{
				RunID: ack.RunID,
				Responses: []protocol.InterruptResponse{{
					ItemID: interrupt.ItemID,
					Response: protocol.InterruptResponseValue{
						Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Blue"}},
					},
				}},
			}, meta, "question-resume", namespace,
		)
		if resumed.RunID != ack.RunID || resumed.SegmentID == ack.SegmentID {
			t.Fatalf("question resume ack = %+v, start = %+v", resumed, ack)
		}
		assertCompletedRunStream(t, ack.RunID, resumedEvents)
		assertFinishedRun(t, runtime.baseURL, ack.RunID, meta)
	})

	t.Run("approval", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "approval")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_APPROVAL"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "approval-run", namespace,
		)
		assertInterruptedRunStream(t, ack.RunID, events)
		pending := rpcCallWithMeta[*protocol.Page[protocol.PendingInterruptSet]](
			t, runtime.baseURL, "interrupts.list",
			protocol.ListInterruptsRequest{SessionID: session.ID, RootRunID: ack.RunID}, meta,
		)
		if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 ||
			pending.Data[0].Interrupts[0].Type != protocol.InterruptApproval {
			t.Fatalf("pending approval = %+v", pending.Data)
		}
		interrupt := pending.Data[0].Interrupts[0]
		_, resumedEvents := rpcRunStream[*protocol.ResumeRunResponse](
			t, runtime.baseURL, "runs.resume", protocol.ResumeRunRequest{
				RunID: ack.RunID,
				Responses: []protocol.InterruptResponse{{
					ItemID: interrupt.ItemID,
					Response: protocol.InterruptResponseValue{
						Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
					},
				}},
			}, meta, "approval-resume", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, resumedEvents)
		assertFinishedRun(t, runtime.baseURL, ack.RunID, meta)
	})

	t.Run("delegation", func(t *testing.T) {
		session := createRunSession(t, runtime.baseURL, namespace, "delegation")
		ack, events := rpcRunStream[*protocol.StartRunResponse](
			t, runtime.baseURL, "runs.start", protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_DELEGATE"}},
				Provider:  "openai-compatible", Model: "test-model",
			}, meta, "delegation-run", namespace,
		)
		assertCompletedRunStream(t, ack.RunID, events)
		runs := rpcCallWithMeta[*protocol.Page[protocol.RunRef]](
			t, runtime.baseURL, "runs.list", protocol.ListRunsRequest{
				SessionID: session.ID, IncludeDescendants: true, PageQuery: protocol.PageQuery{Limit: 100},
			}, meta,
		)
		if len(runs.Data) != 2 {
			t.Fatalf("delegated Run tree = %+v", runs.Data)
		}
		childFound := false
		for _, run := range runs.Data {
			if run.ID != ack.RunID && run.ParentRunID == ack.RunID && run.RootRunID == ack.RunID {
				childFound = true
			}
		}
		if !childFound {
			t.Fatalf("delegated Run tree lacks child lineage: %+v", runs.Data)
		}
	})

	if model.calls.Load() == 0 {
		t.Fatal("model server received no calls")
	}
}

func createRunSession(t *testing.T, baseURL string, namespace string, title string) *protocol.Session {
	t.Helper()
	return rpcCall[*protocol.Session](
		t, baseURL, "sessions.create", protocol.CreateSessionRequest{
			Title: title, Provider: "openai-compatible", Model: "test-model",
		}, "session-"+title, namespace,
	)
}

func rpcCallWithMeta[Result any](
	t *testing.T,
	baseURL string,
	method string,
	params any,
	meta protocol.RequestMeta,
) Result {
	t.Helper()
	response := rpcRequestWithMeta(t, baseURL, method, params, &meta, "", "")
	if response.Error != nil {
		t.Fatalf("%s problem = %+v", method, response.Error.Data)
	}
	var value Result
	if err := json.Unmarshal(response.Result, &value); err != nil {
		t.Fatalf("decode %s result error = %v", method, err)
	}
	return value
}

func rpcRunStream[Ack any](
	t *testing.T,
	baseURL string,
	method string,
	params any,
	meta protocol.RequestMeta,
	idempotencyKey string,
	idempotencyNamespace string,
) (Ack, []protocol.RunEvent) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "stream-acceptance", "method": method,
		"params": rpcParameters(t, params, &meta),
	})
	if err != nil {
		t.Fatalf("encode %s request error = %v", method, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, baseURL+httptransport.PathRPC, bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build %s request error = %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.Header.Set("Idempotency-Namespace", idempotencyNamespace)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call %s stream error = %v", method, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("call %s stream status = %d", method, response.StatusCode)
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
			var envelope rpcResponse
			if err := json.Unmarshal(message.Data, &envelope); err != nil {
				t.Fatalf("decode %s ack envelope error = %v", method, err)
			}
			if envelope.Error != nil {
				t.Fatalf("%s ack problem = %+v", method, envelope.Error.Data)
			}
			if err := json.Unmarshal(envelope.Result, &ack); err != nil {
				t.Fatalf("decode %s ack error = %v", method, err)
			}
			continue
		}
		var notification struct {
			Method string            `json:"method"`
			Params protocol.RunEvent `json:"params"`
		}
		if err := json.Unmarshal(message.Data, &notification); err != nil {
			t.Fatalf("decode %s notification error = %v", method, err)
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

func assertCompletedRunStream(t *testing.T, runID string, events []protocol.RunEvent) {
	t.Helper()
	started, finished := false, false
	for _, event := range events {
		if event.RunID != runID {
			continue
		}
		switch event.Event.Type {
		case protocol.StreamSegmentStarted:
			started = true
		case protocol.StreamSegmentFinished:
			finished = event.Event.Outcome != nil && event.Event.Outcome.Type == protocol.SegmentCompleted
		}
	}
	if !started || !finished {
		t.Fatalf("completed stream flags = started:%v finished:%v events:%+v", started, finished, events)
	}
}

func assertInterruptedRunStream(t *testing.T, runID string, events []protocol.RunEvent) {
	t.Helper()
	if streamSettledWith(events, runID, protocol.SegmentInterrupt) {
		return
	}
	t.Fatalf("Run %q stream omitted interrupt settlement: %+v", runID, events)
}

func streamSettledWith(events []protocol.RunEvent, runID string, outcome protocol.SegmentOutcomeType) bool {
	for _, event := range events {
		if event.RunID == runID && event.Event.Type == protocol.StreamSegmentFinished &&
			event.Event.Outcome != nil && event.Event.Outcome.Type == outcome {
			return true
		}
	}
	return false
}

func assertFinishedRun(t *testing.T, baseURL string, runID string, meta protocol.RequestMeta) {
	t.Helper()
	run := rpcCallWithMeta[*protocol.RunRef](
		t, baseURL, "runs.get", protocol.GetRunRequest{RunID: runID}, meta,
	)
	if run.Status != protocol.RunStatusFinished || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run %q state = %+v", runID, run)
	}
}

func containsFinalText(items []protocol.Item, wanted string) bool {
	for _, item := range items {
		if item.Type != protocol.ItemTypeAgentMessage || item.Phase != protocol.MessagePhaseFinalAnswer {
			continue
		}
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText && strings.Contains(block.Text, wanted) {
				return true
			}
		}
	}
	return false
}

type scenarioModel struct {
	*httptest.Server
	calls atomic.Int64
}

func newScenarioModel(t *testing.T) *scenarioModel {
	t.Helper()
	model := &scenarioModel{}
	model.Server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("model authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/models":
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprint(response, `{"object":"list","data":[{"id":"test-model","object":"model"}]}`)
		case "/chat/completions":
			model.calls.Add(1)
			var body struct {
				Stream   bool `json:"stream"`
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode model request error = %v", err)
				http.Error(response, "bad request", http.StatusBadRequest)
				return
			}
			encoded, _ := json.Marshal(body.Messages)
			material := string(encoded)
			if !body.Stream {
				writeScenarioCompletion(response, "background complete")
				return
			}
			hasToolResult := false
			for _, message := range body.Messages {
				hasToolResult = hasToolResult || message.Role == "tool"
			}
			switch {
			case strings.Contains(material, "SCENARIO_CHILD"):
				writeScenarioTextStream(response, "child complete")
			case strings.Contains(material, "SCENARIO_QUESTION") && !hasToolResult:
				writeScenarioToolStream(response, "call-question", "ask_user", `{"fields":[{"prompt":"Pick a color","header":"Color","type":"choice","options":[{"label":"Blue"},{"label":"Green"}]}]}`)
			case strings.Contains(material, "SCENARIO_QUESTION"):
				writeScenarioTextStream(response, "question complete")
			case strings.Contains(material, "SCENARIO_APPROVAL") && !hasToolResult:
				writeScenarioToolStream(response, "call-shell", "shell", `{"command":"printf r11-approved","description":"Verify approval path"}`)
			case strings.Contains(material, "SCENARIO_APPROVAL"):
				writeScenarioTextStream(response, "approval complete")
			case strings.Contains(material, "SCENARIO_DELEGATE") && !hasToolResult:
				writeScenarioToolStream(response, "call-delegate", "delegate_task", `{"summary":"Independent child","instructions":"SCENARIO_CHILD"}`)
			case strings.Contains(material, "SCENARIO_DELEGATE"):
				writeScenarioTextStream(response, "delegation complete")
			default:
				writeScenarioTextStream(response, "normal complete")
			}
		default:
			http.NotFound(response, request)
		}
	}))
	return model
}

func writeScenarioTextStream(response http.ResponseWriter, content string) {
	writeScenarioStream(response, map[string]any{
		"role": "assistant", "content": content,
	}, "stop")
}

func writeScenarioToolStream(response http.ResponseWriter, id string, name string, arguments string) {
	writeScenarioStream(response, map[string]any{
		"role": "assistant",
		"tool_calls": []any{map[string]any{
			"index": 0, "id": id, "type": "function",
			"function": map[string]any{"name": name, "arguments": arguments},
		}},
	}, "tool_calls")
}

func writeScenarioStream(response http.ResponseWriter, delta map[string]any, finishReason string) {
	response.Header().Set("Content-Type", "text/event-stream")
	chunk := map[string]any{
		"id": "chatcmpl-acceptance", "object": "chat.completion.chunk",
		"created": 1787529600, "model": "test-model",
		"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": finishReason,
		}},
	}
	encoded, _ := json.Marshal(chunk)
	fmt.Fprintf(response, "data: %s\n\n", encoded)
	usage, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-acceptance", "object": "chat.completion.chunk",
		"created": 1787529600, "model": "test-model", "choices": []any{},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
	})
	fmt.Fprintf(response, "data: %s\n\ndata: [DONE]\n\n", usage)
}

func writeScenarioCompletion(response http.ResponseWriter, content string) {
	response.Header().Set("Content-Type", "application/json")
	encoded, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-acceptance", "object": "chat.completion", "created": 1787529600,
		"model": "test-model",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
	})
	_, _ = response.Write(encoded)
}
