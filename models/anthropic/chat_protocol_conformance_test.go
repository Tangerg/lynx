package anthropic_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/internal/conformance"
)

func TestChat_CoreConformance(t *testing.T) {
	conformance.ChatSuite{
		New: func(t *testing.T) (corechat.Model, corechat.Streamer) {
			t.Helper()
			server := newProtocolChatServer(t)
			t.Cleanup(server.Close)
			adapter, err := anthropic.NewChat(anthropic.ChatConfig{
				APIKey:         "test-key",
				DefaultOptions: corechat.Options{Model: "default-must-be-overridden"},
				RequestOptions: []option.RequestOption{option.WithBaseURL(server.URL)},
			})
			if err != nil {
				t.Fatalf("NewChat: %v", err)
			}
			return adapter, adapter
		},
		Request: newProtocolChatRequest,
		AssertCall: func(t *testing.T, response *corechat.Response) {
			t.Helper()
			if response.ID != "msg-1" || response.Model != "claude-opus-4-6" {
				t.Fatalf("identity = %q/%q", response.ID, response.Model)
			}
			if len(response.Choices) != 1 {
				t.Fatalf("choices = %d; want 1", len(response.Choices))
			}
			choice := response.Choices[0]
			if choice.FinishReason != corechat.FinishReasonToolCalls || choice.Message == nil {
				t.Fatalf("choice = %#v", choice)
			}
			if len(choice.Message.Parts) != 3 {
				t.Fatalf("parts = %#v", choice.Message.Parts)
			}
			reasoning := choice.Message.Parts[0]
			if reasoning.Kind != corechat.PartReasoning || reasoning.Text != "compare the evidence" || string(reasoning.Signature) != "sig-response" {
				t.Errorf("reasoning = %#v", reasoning)
			}
			call := choice.Message.Parts[2].ToolCall
			if call == nil || call.ID != "toolu-2" || call.Name != "lookup" || call.Arguments != `{"id":8}` {
				t.Errorf("tool call = %#v", call)
			}
			redacted, ok, err := metadata.Decode[string](choice.Message.Metadata, "anthropic/redacted_reasoning")
			if err != nil || !ok || redacted != "opaque-redacted-block" {
				t.Errorf("redacted reasoning = %q/%v/%v", redacted, ok, err)
			}
			if response.Usage.InputTokens != 160 || response.Usage.OutputTokens != 30 ||
				response.Usage.ReasoningTokens == nil || *response.Usage.ReasoningTokens != 10 ||
				response.Usage.CacheReadInputTokens == nil || *response.Usage.CacheReadInputTokens != 40 ||
				response.Usage.CacheWriteInputTokens == nil || *response.Usage.CacheWriteInputTokens != 20 {
				t.Errorf("usage = %#v", response.Usage)
			}
		},
		AssertStream: func(t *testing.T, responses []*corechat.Response) {
			t.Helper()
			var text, reasoning, signature strings.Builder
			var toolIDs []string
			var sawRedacted bool
			var finalUsage corechat.Usage
			for _, response := range responses {
				finalUsage = response.Usage
				for i := range response.Choices {
					message := response.Choices[i].Message
					if message == nil {
						continue
					}
					if value, ok, err := metadata.Decode[string](message.Metadata, "anthropic/redacted_reasoning"); err == nil && ok && value == "opaque-stream" {
						sawRedacted = true
					}
					for _, part := range message.Parts {
						switch part.Kind {
						case corechat.PartText:
							text.WriteString(part.Text)
						case corechat.PartReasoning:
							reasoning.WriteString(part.Text)
							signature.Write(part.Signature)
						case corechat.PartToolCall:
							toolIDs = append(toolIDs, part.ToolCall.ID)
						}
					}
				}
			}
			if text.String() != "need another lookup" || reasoning.String() != "compare evidence" || signature.String() != "sig-stream" {
				t.Errorf("stream text/reasoning/signature = %q/%q/%q", text.String(), reasoning.String(), signature.String())
			}
			if !sawRedacted {
				t.Error("stream did not attach redacted reasoning to the next message delta")
			}
			if len(toolIDs) != 3 {
				t.Fatalf("tool deltas = %v", toolIDs)
			}
			for _, id := range toolIDs {
				if id != "toolu-stream" {
					t.Errorf("unstable tool ID %q", id)
				}
			}
			if finalUsage.InputTokens != 160 || finalUsage.OutputTokens != 30 || finalUsage.ReasoningTokens == nil || *finalUsage.ReasoningTokens != 10 {
				t.Errorf("final usage = %#v", finalUsage)
			}
		},
		AssertAggregated: func(t *testing.T, response *corechat.Response) {
			t.Helper()
			if response.ID != "msg-stream" || response.Model != "claude-opus-4-6" || len(response.Choices) != 1 {
				t.Fatalf("aggregated identity/choices = %q/%q/%d", response.ID, response.Model, len(response.Choices))
			}
			choice := response.Choices[0]
			if choice.Message == nil || len(choice.Message.Parts) != 3 || choice.FinishReason != corechat.FinishReasonToolCalls {
				t.Fatalf("aggregated choice = %#v", choice)
			}
			reasoning := choice.Message.Parts[0]
			call := choice.Message.Parts[2].ToolCall
			if reasoning.Text != "compare evidence" || string(reasoning.Signature) != "sig-stream" || choice.Message.Parts[1].Text != "need another lookup" ||
				call == nil || call.ID != "toolu-stream" || call.Arguments != `{"id":9}` {
				t.Errorf("aggregated parts = %#v; call = %#v", choice.Message.Parts, call)
			}
			if redacted, found, err := metadata.Decode[string](choice.Message.Metadata, "anthropic/redacted_reasoning"); err != nil || !found || redacted != "opaque-stream" {
				t.Errorf("aggregated redacted reasoning = %q/%v/%v", redacted, found, err)
			}
			if response.Usage.InputTokens != 160 || response.Usage.OutputTokens != 30 {
				t.Errorf("aggregated usage = %#v", response.Usage)
			}
		},
	}.Run(t)
}

func newProtocolChatRequest(t *testing.T) *corechat.Request {
	t.Helper()
	image, err := media.NewURI("image/jpeg", "https://example.com/image.jpg")
	if err != nil {
		t.Fatalf("NewURI: %v", err)
	}
	pdf, err := media.NewBytes("application/pdf", []byte("pdf"))
	if err != nil {
		t.Fatalf("NewBytes: %v", err)
	}
	pdf.Name = "paper.pdf"

	assistant := corechat.NewAssistantMessage(
		corechat.NewReasoningPart("need a lookup", []byte("sig-anthropic")),
		corechat.NewTextPart("I need one fact."),
		corechat.NewToolCallPart(corechat.ToolCall{ID: "toolu-1", Name: "lookup", Arguments: `{"id":7}`}),
	)
	assistant.Metadata = metadata.New()
	if err := assistant.Metadata.Set("anthropic/redacted_reasoning", "prior-opaque-block"); err != nil {
		t.Fatalf("set redacted reasoning: %v", err)
	}

	request, err := corechat.NewRequest(
		corechat.NewSystemMessage("Follow policy."),
		corechat.NewUserMessage(corechat.NewTextPart("Read the image and PDF."), corechat.NewMediaPart(image), corechat.NewMediaPart(pdf)),
		assistant,
		corechat.NewToolMessage(corechat.ToolResult{ID: "toolu-1", Name: "lookup", Result: "not found", IsError: true}),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	maxTokens := int64(1024)
	temperature := 0.2
	topK := int64(40)
	topP := 0.95
	request.Options = corechat.Options{
		Model:       "claude-opus-4-6",
		MaxTokens:   &maxTokens,
		Stop:        []string{"END"},
		Temperature: &temperature,
		TopK:        &topK,
		TopP:        &topP,
	}
	request.Tools = []corechat.ToolDefinition{{
		Name:        "lookup",
		Description: "Look up a record",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"}}}`),
	}}
	if err := request.SetExtension("anthropic/request", map[string]any{
		"thinking": map[string]any{"type": "enabled", "budget_tokens": 512},
	}); err != nil {
		t.Fatalf("SetExtension: %v", err)
	}
	return request
}

func newProtocolChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Model     string `json:"model"`
			MaxTokens int64  `json:"max_tokens"`
			Stream    bool   `json:"stream"`
			Thinking  struct {
				Type         string `json:"type"`
				BudgetTokens int64  `json:"budget_tokens"`
			} `json:"thinking"`
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type   string `json:"type"`
					Title  string `json:"title"`
					Source struct {
						Type string `json:"type"`
						URL  string `json:"url"`
						Data string `json:"data"`
					} `json:"source"`
					CacheControl struct {
						Type string `json:"type"`
					} `json:"cache_control"`
					Signature string `json:"signature"`
					Data      string `json:"data"`
					IsError   bool   `json:"is_error"`
					ToolUseID string `json:"tool_use_id"`
				} `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Name         string `json:"name"`
				CacheControl struct {
					Type string `json:"type"`
				} `json:"cache_control"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		if request.URL.Path != "/v1/messages" || request.Header.Get("x-api-key") != "test-key" || request.Header.Get("anthropic-version") == "" {
			t.Errorf("request identity = %q/%q/%q", request.URL.Path, request.Header.Get("x-api-key"), request.Header.Get("anthropic-version"))
		}
		if body.Model != "claude-opus-4-6" || body.MaxTokens != 1024 || len(body.System) != 1 || len(body.Messages) != 3 || len(body.Tools) != 1 {
			t.Errorf("request shape = model %q max %d system %d messages %d tools %d", body.Model, body.MaxTokens, len(body.System), len(body.Messages), len(body.Tools))
		}
		if body.Thinking.Type != "enabled" || body.Thinking.BudgetTokens != 512 {
			t.Errorf("thinking = %#v", body.Thinking)
		}
		user := body.Messages[0].Content
		if len(user) != 3 || user[1].Type != "image" || user[1].Source.Type != "url" || user[1].Source.URL == "" ||
			user[2].Type != "document" || user[2].Source.Type != "base64" || user[2].Source.Data == "" || user[2].Title != "paper.pdf" {
			t.Errorf("user media blocks = %#v", user)
		}
		assistant := body.Messages[1].Content
		if len(assistant) != 4 || assistant[0].Type != "redacted_thinking" || assistant[0].Data != "prior-opaque-block" || assistant[1].Signature != "sig-anthropic" {
			t.Errorf("assistant blocks = %#v", assistant)
		}
		toolResult := body.Messages[2].Content
		if len(toolResult) != 1 || toolResult[0].Type != "tool_result" || !toolResult[0].IsError || toolResult[0].ToolUseID != "toolu-1" {
			t.Errorf("tool result = %#v", toolResult)
		}
		if body.Tools[0].CacheControl.Type != "ephemeral" || toolResult[0].CacheControl.Type != "ephemeral" {
			t.Errorf("automatic cache breakpoints = tool %q/message %q", body.Tools[0].CacheControl.Type, toolResult[0].CacheControl.Type)
		}
		if body.Stream {
			writeProtocolChatStream(writer)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, protocolChatResponseJSON)
	}))
}

func writeProtocolChatStream(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	events := []struct {
		name string
		data string
	}{
		{"message_start", `{"type":"message_start","message":{"id":"msg-stream","type":"message","role":"assistant","model":"claude-opus-4-6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":40,"cache_creation_input_tokens":20}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"compare evidence"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-stream"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"opaque-stream"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"need another "}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"lookup"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":2}`},
		{"content_block_start", `{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu-stream","name":"lookup","input":{}}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"id\":"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"9}"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":3}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":"END"},"usage":{"output_tokens":30,"output_tokens_details":{"thinking_tokens":10}}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
	for _, event := range events {
		fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.name, event.data)
	}
}

const protocolChatResponseJSON = `{
  "id":"msg-1",
  "type":"message",
  "role":"assistant",
  "model":"claude-opus-4-6",
  "content":[
    {"type":"thinking","thinking":"compare the evidence","signature":"sig-response"},
    {"type":"redacted_thinking","data":"opaque-redacted-block"},
    {"type":"text","text":"I need another lookup."},
    {"type":"tool_use","id":"toolu-2","name":"lookup","input":{"id":8}}
  ],
  "stop_reason":"tool_use",
  "stop_sequence":"END",
  "usage":{
    "input_tokens":100,
    "output_tokens":30,
    "cache_read_input_tokens":40,
    "cache_creation_input_tokens":20,
    "output_tokens_details":{"thinking_tokens":10}
  }
}`
