package ollama_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/models/internal/conformance"
	"github.com/Tangerg/lynx/models/ollama"
)

func TestChat_CoreConformance(t *testing.T) {
	conformance.ChatSuite{
		New: func(t *testing.T) (corechat.Model, corechat.Streamer) {
			t.Helper()
			server := newProtocolChatServer(t)
			t.Cleanup(server.Close)
			adapter, err := ollama.NewChat(ollama.ChatConfig{
				DefaultOptions: corechat.Options{Model: "default-must-be-overridden"},
				BaseURL:        server.URL,
			})
			if err != nil {
				t.Fatalf("NewChat: %v", err)
			}
			return adapter, adapter
		},
		Request: newProtocolChatRequest,
		AssertCall: func(t *testing.T, response *corechat.Response) {
			t.Helper()
			assertProtocolResponse(t, response)
		},
		AssertStream: func(t *testing.T, responses []*corechat.Response) {
			t.Helper()
			var reasoning, content strings.Builder
			var toolCall *corechat.ToolCall
			var final *corechat.Response
			for _, response := range responses {
				final = response
				for i := range response.Choices {
					message := response.Choices[i].Message
					if message == nil {
						continue
					}
					for j := range message.Parts {
						part := message.Parts[j]
						switch part.Kind {
						case corechat.PartReasoning:
							reasoning.WriteString(part.Text)
						case corechat.PartText:
							content.WriteString(part.Text)
						case corechat.PartToolCall:
							toolCall = part.ToolCall
						}
					}
				}
			}
			if reasoning.String() != "inspect colors" || content.String() != "It is a blue square." {
				t.Errorf("stream reasoning/text = %q/%q", reasoning.String(), content.String())
			}
			if toolCall == nil || toolCall.ID != "ollama/0/2" || toolCall.Name != "inspect" || toolCall.Arguments != `{"detail":true}` {
				t.Errorf("stream tool call = %#v", toolCall)
			}
			if final == nil || final.Usage.InputTokens != 11 || final.Usage.OutputTokens != 5 {
				t.Errorf("final usage = %#v", final)
			}
		},
	}.Run(t)
}

func TestChat_RejectsUnsupportedInputBeforeProviderIO(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hits.Add(1)
		http.Error(writer, "unexpected provider call", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	adapter, err := ollama.NewChat(ollama.ChatConfig{
		DefaultOptions: corechat.Options{Model: "qwen3:8b"},
		BaseURL:        server.URL,
	})
	if err != nil {
		t.Fatalf("NewChat: %v", err)
	}

	uriImage, err := media.NewURI("image/png", "https://example.com/image.png")
	if err != nil {
		t.Fatalf("NewURI: %v", err)
	}
	tests := []struct {
		name    string
		message corechat.Message
		want    string
	}{
		{
			name:    "reasoning signature",
			message: corechat.NewAssistantMessage(corechat.NewReasoningPart("thinking", []byte("opaque"))),
			want:    "reasoning signature is unsupported",
		},
		{
			name:    "URI image",
			message: corechat.NewUserMessage(corechat.NewMediaPart(uriImage)),
			want:    "Ollama requires bytes",
		},
		{
			name: "non-object tool arguments",
			message: corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
				ID: "call-1", Name: "inspect", Arguments: `[true]`,
			})),
			want: "cannot unmarshal array",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := corechat.NewRequest(test.message)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			_, err = adapter.Call(t.Context(), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Call error = %v; want substring %q", err, test.want)
			}
		})
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("provider HTTP calls = %d; want 0", got)
	}
}

func newProtocolChatRequest(t *testing.T) *corechat.Request {
	t.Helper()
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatalf("NewBytes: %v", err)
	}
	request, err := corechat.NewRequest(
		corechat.NewSystemMessage("Be concise."),
		corechat.NewUserMessage(corechat.NewTextPart("Describe this image."), corechat.NewMediaPart(image)),
		corechat.NewAssistantMessage(
			corechat.NewReasoningPart("inspect pixels", nil),
			corechat.NewTextPart("I will inspect it."),
			corechat.NewToolCallPart(corechat.ToolCall{ID: "ollama/0/2", Name: "inspect", Arguments: `{"detail":true}`}),
		),
		corechat.NewToolMessage(corechat.ToolResult{ID: "ollama/0/2", Name: "inspect", Result: "blue square"}),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	frequencyPenalty := 0.1
	maxTokens := int64(256)
	presencePenalty := 0.1
	temperature := 0.5
	topK := int64(20)
	topP := 0.8
	request.Options = corechat.Options{
		Model:            "qwen3:8b",
		FrequencyPenalty: &frequencyPenalty,
		MaxTokens:        &maxTokens,
		PresencePenalty:  &presencePenalty,
		Stop:             []string{"END"},
		Temperature:      &temperature,
		TopK:             &topK,
		TopP:             &topP,
	}
	request.Tools = []corechat.ToolDefinition{{
		Name:        "inspect",
		Description: "Inspect image details",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"detail":{"type":"boolean"}}}`),
	}}
	if err := request.SetExtension("ollama/request", map[string]any{
		"keep_alive": "10m",
		"format":     "json",
		"think":      true,
		"options": map[string]any{
			"seed":        42,
			"num_ctx":     8192,
			"temperature": 1.5,
		},
	}); err != nil {
		t.Fatalf("SetExtension: %v", err)
	}
	return request
}

func newProtocolChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/chat" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		var body protocolChatRequestWire
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		assertProtocolRequestWire(t, body)
		writer.Header().Set("Content-Type", "application/x-ndjson")
		if body.Stream != nil && *body.Stream {
			writeProtocolChatStream(writer)
			return
		}
		fmt.Fprintln(writer, protocolChatResponseJSON)
	}))
}

type protocolChatRequestWire struct {
	Model     string                `json:"model"`
	Messages  []protocolMessageWire `json:"messages"`
	Stream    *bool                 `json:"stream"`
	Format    string                `json:"format"`
	KeepAlive string                `json:"keep_alive"`
	Think     bool                  `json:"think"`
	Options   map[string]any        `json:"options"`
	Tools     []struct {
		Type     string `json:"type"`
		Function struct {
			Name       string `json:"name"`
			Parameters struct {
				Type       string         `json:"type"`
				Properties map[string]any `json:"properties"`
			} `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
}

type protocolMessageWire struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	Thinking   string   `json:"thinking"`
	Images     []string `json:"images"`
	ToolName   string   `json:"tool_name"`
	ToolCallID string   `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func assertProtocolRequestWire(t *testing.T, body protocolChatRequestWire) {
	t.Helper()
	if body.Model != "qwen3:8b" || body.Stream == nil || body.Format != "json" || body.KeepAlive != "10m0s" || !body.Think {
		t.Errorf("request identity/native config = %#v", body)
	}
	if len(body.Messages) != 4 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" || body.Messages[2].Role != "assistant" || body.Messages[3].Role != "tool" {
		t.Fatalf("messages = %#v", body.Messages)
	}
	if body.Messages[1].Content != "Describe this image." || len(body.Messages[1].Images) != 1 || body.Messages[1].Images[0] != "aW1hZ2U=" {
		t.Errorf("user message = %#v", body.Messages[1])
	}
	assistant := body.Messages[2]
	if assistant.Thinking != "inspect pixels" || assistant.Content != "I will inspect it." || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "ollama/0/2" || assistant.ToolCalls[0].Function.Arguments["detail"] != true {
		t.Errorf("assistant message = %#v", assistant)
	}
	tool := body.Messages[3]
	if tool.Content != "blue square" || tool.ToolName != "inspect" || tool.ToolCallID != "ollama/0/2" {
		t.Errorf("tool message = %#v", tool)
	}
	if len(body.Tools) != 1 || body.Tools[0].Type != "function" || body.Tools[0].Function.Name != "inspect" || body.Tools[0].Function.Parameters.Type != "object" || len(body.Tools[0].Function.Parameters.Properties) != 1 {
		t.Errorf("tools = %#v", body.Tools)
	}
	if body.Options["seed"] != float64(42) || body.Options["num_ctx"] != float64(8192) || body.Options["temperature"] != float64(0.5) ||
		body.Options["num_predict"] != float64(256) || body.Options["top_k"] != float64(20) || body.Options["top_p"] != float64(0.8) ||
		body.Options["frequency_penalty"] != float64(0.1) || body.Options["presence_penalty"] != float64(0.1) {
		t.Errorf("options = %#v", body.Options)
	}
}

func assertProtocolResponse(t *testing.T, response *corechat.Response) {
	t.Helper()
	if response.Model != "qwen3:8b" || len(response.Choices) != 1 {
		t.Fatalf("response identity/choices = %q/%d", response.Model, len(response.Choices))
	}
	choice := response.Choices[0]
	if choice.Message == nil || len(choice.Message.Parts) != 3 || choice.FinishReason != corechat.FinishReasonStop {
		t.Fatalf("choice = %#v", choice)
	}
	if choice.Message.Parts[0].Kind != corechat.PartReasoning || choice.Message.Parts[0].Text != "inspect colors" ||
		choice.Message.Parts[1].Kind != corechat.PartText || choice.Message.Parts[1].Text != "It is a blue square." {
		t.Errorf("reasoning/text = %#v", choice.Message.Parts)
	}
	call := choice.Message.Parts[2].ToolCall
	if call == nil || call.ID != "ollama/0/2" || call.Name != "inspect" || call.Arguments != `{"detail":true}` {
		t.Errorf("tool call = %#v", call)
	}
	if response.Usage.InputTokens != 11 || response.Usage.OutputTokens != 5 {
		t.Errorf("usage = %#v", response.Usage)
	}
	createdAt := decodeExtension[string](t, response.Extensions, "ollama/created_at")
	if createdAt != "2026-07-14T12:00:00Z" {
		t.Errorf("created_at = %q", createdAt)
	}
	durations := decodeExtension[map[string]int64](t, response.Extensions, "ollama/durations_ms")
	if durations["total"] != 1250 || durations["load"] != 100 || durations["prompt_eval"] != 300 || durations["eval"] != 700 {
		t.Errorf("durations = %#v", durations)
	}
	metrics := decodeExtension[map[string]int](t, response.Extensions, "ollama/metrics")
	if metrics["prompt_eval_count"] != 11 || metrics["eval_count"] != 5 {
		t.Errorf("metrics = %#v", metrics)
	}
	nativeReason := decodeExtension[string](t, choice.Extensions, "ollama/native_done_reason")
	if nativeReason != "stop" {
		t.Errorf("native done reason = %q", nativeReason)
	}
}

func decodeExtension[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, found, err := metadata.Decode[T](values, key)
	if err != nil {
		t.Fatalf("decode extension %q: %v", key, err)
	}
	if !found {
		t.Fatalf("extension %q not found", key)
	}
	return value
}

func writeProtocolChatStream(writer http.ResponseWriter) {
	chunks := []string{
		`{"model":"qwen3:8b","message":{"role":"assistant","thinking":"inspect colors"},"done":false}`,
		`{"model":"qwen3:8b","message":{"role":"assistant","content":"It is a blue square."},"done":false}`,
		`{"model":"qwen3:8b","message":{"role":"assistant","tool_calls":[{"function":{"name":"inspect","arguments":{"detail":true}}}]},"done":false}`,
		`{"model":"qwen3:8b","created_at":"2026-07-14T12:00:00Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","total_duration":1250000000,"load_duration":100000000,"prompt_eval_count":11,"prompt_eval_duration":300000000,"eval_count":5,"eval_duration":700000000}`,
	}
	for _, chunk := range chunks {
		fmt.Fprintln(writer, chunk)
	}
}

const protocolChatResponseJSON = `{"model":"qwen3:8b","created_at":"2026-07-14T12:00:00Z","message":{"role":"assistant","thinking":"inspect colors","content":"It is a blue square.","tool_calls":[{"function":{"name":"inspect","arguments":{"detail":true}}}]},"done":true,"done_reason":"stop","total_duration":1250000000,"load_duration":100000000,"prompt_eval_count":11,"prompt_eval_duration":300000000,"eval_count":5,"eval_duration":700000000}`
