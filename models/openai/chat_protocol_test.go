package openai_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/models/internal/conformance"
	lynxopenai "github.com/Tangerg/lynx/models/openai"
)

func TestChat_CoreConformance(t *testing.T) {
	conformance.ChatSuite{
		New: func(t *testing.T) (corechat.Model, corechat.Streamer) {
			t.Helper()
			server := newCoreChatServer(t)
			t.Cleanup(server.Close)
			adapter, err := lynxopenai.NewChat(lynxopenai.ChatConfig{
				APIKey:         "test-key",
				DefaultOptions: corechat.Options{Model: "gpt-default-must-be-overridden"},
				RequestOptions: []option.RequestOption{option.WithBaseURL(server.URL)},
			})
			if err != nil {
				t.Fatalf("NewChat: %v", err)
			}
			return adapter, adapter
		},
		Request: newCoreChatRequest,
		AssertCall: func(t *testing.T, response *corechat.Response) {
			t.Helper()
			if response.ID != "chatcmpl-core" || response.Model != "gpt-5.2" {
				t.Fatalf("identity = %q/%q", response.ID, response.Model)
			}
			if len(response.Choices) != 2 {
				t.Fatalf("choices = %d; want 2", len(response.Choices))
			}
			first := response.Choices[0]
			if first.FinishReason != corechat.FinishReasonToolCalls {
				t.Errorf("finish reason = %q", first.FinishReason)
			}
			if first.Message == nil || len(first.Message.Parts) != 4 {
				t.Fatalf("first message = %#v; want reasoning/text/tool/media", first.Message)
			}
			if first.Message.Parts[0].Kind != corechat.PartReasoning || first.Message.Parts[0].Text != "checking sources" {
				t.Errorf("reasoning part = %#v", first.Message.Parts[0])
			}
			if audioID, ok, err := metadata.Decode[string](first.Message.Metadata, "openai/audio_id"); err != nil || !ok || audioID != "audio-1" {
				t.Errorf("audio replay metadata = %q/%v/%v", audioID, ok, err)
			}
			call := first.Message.Parts[2].ToolCall
			if call == nil || call.ID != "call-2" || call.Name != "search" {
				t.Errorf("tool call = %#v", call)
			}
			audio := first.Message.Parts[3].Media
			if audio == nil || audio.MIME != "audio/wav" || audio.Source.Kind != media.SourceReference || audio.Source.Ref != "audio-1" {
				t.Errorf("audio = %#v", audio)
			}
			if response.Usage.InputTokens != 12 || response.Usage.OutputTokens != 7 ||
				response.Usage.ReasoningTokens == nil || *response.Usage.ReasoningTokens != 3 ||
				response.Usage.CacheReadInputTokens == nil || *response.Usage.CacheReadInputTokens != 5 {
				t.Errorf("usage = %#v", response.Usage)
			}
			if response.Choices[1].Text() != "alternate" {
				t.Errorf("alternate text = %q", response.Choices[1].Text())
			}
		},
		AssertStream: func(t *testing.T, responses []*corechat.Response) {
			t.Helper()
			var text, reasoning strings.Builder
			var toolIDs []string
			var finalUsage corechat.Usage
			for _, response := range responses {
				finalUsage = response.Usage
				for i := range response.Choices {
					if response.Choices[i].Message == nil {
						continue
					}
					for _, part := range response.Choices[i].Message.Parts {
						switch part.Kind {
						case corechat.PartText:
							text.WriteString(part.Text)
						case corechat.PartReasoning:
							reasoning.WriteString(part.Text)
						case corechat.PartToolCall:
							toolIDs = append(toolIDs, part.ToolCall.ID)
						}
					}
				}
			}
			if text.String() != "hello world" || reasoning.String() != "think " {
				t.Errorf("stream text/reasoning = %q/%q", text.String(), reasoning.String())
			}
			if len(toolIDs) != 2 {
				t.Fatalf("tool deltas = %v", toolIDs)
			}
			for _, id := range toolIDs {
				if id != "call-stream" {
					t.Errorf("unstable tool ID %q", id)
				}
			}
			if finalUsage.InputTokens != 8 || finalUsage.OutputTokens != 4 {
				t.Errorf("final usage = %#v", finalUsage)
			}
		},
		AssertAggregated: func(t *testing.T, response *corechat.Response) {
			t.Helper()
			if response.ID != "chatcmpl-stream" || response.Model != "gpt-5.2" || len(response.Choices) != 1 {
				t.Fatalf("aggregated identity/choices = %q/%q/%d", response.ID, response.Model, len(response.Choices))
			}
			choice := response.Choices[0]
			if choice.Message == nil || len(choice.Message.Parts) != 3 || choice.FinishReason != corechat.FinishReasonToolCalls {
				t.Fatalf("aggregated choice = %#v", choice)
			}
			call := choice.Message.Parts[2].ToolCall
			if choice.Message.Parts[0].Text != "think " || choice.Message.Parts[1].Text != "hello world" || call == nil || call.Arguments != `{"q":"lynx"}` {
				t.Errorf("aggregated parts = %#v; call = %#v", choice.Message.Parts, call)
			}
			if response.Usage.InputTokens != 8 || response.Usage.OutputTokens != 4 {
				t.Errorf("aggregated usage = %#v", response.Usage)
			}
		},
	}.Run(t)
}

func newCoreChatRequest(t *testing.T) *corechat.Request {
	t.Helper()
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatalf("NewBytes: %v", err)
	}
	image.Name = "diagram.png"
	file, err := media.NewReference("application/pdf", "file-123")
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	file.Name = "spec.pdf"

	assistant := corechat.NewAssistantMessage(
		corechat.NewTextPart("I will search."),
		corechat.NewToolCallPart(corechat.ToolCall{ID: "call-1", Name: "search", Arguments: `{"q":"lynx"}`}),
	)
	assistant.Metadata = metadata.New()
	if err := assistant.Metadata.Set("openai/audio_id", "audio-prev"); err != nil {
		t.Fatalf("set audio metadata: %v", err)
	}
	if err := assistant.Metadata.Set("openai/refusal", ""); err != nil {
		t.Fatalf("set refusal metadata: %v", err)
	}

	request, err := corechat.NewRequest(
		corechat.NewSystemMessage("You are precise."),
		corechat.NewUserMessage(corechat.NewTextPart("Inspect these inputs."), corechat.NewMediaPart(image), corechat.NewMediaPart(file)),
		assistant,
		corechat.NewToolMessage(corechat.ToolResult{ID: "call-1", Name: "search", Result: `{"hits":2}`}),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	temperature := 0.3
	maxTokens := int64(512)
	request.Options = corechat.Options{Model: "gpt-5.2", Temperature: &temperature, MaxTokens: &maxTokens, Stop: []string{"<END>"}}
	request.Tools = []corechat.ToolDefinition{{
		Name:        "search",
		Description: "Search the index",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
	}}
	if err := request.SetExtension("openai/request", map[string]any{
		"modalities": []string{"text", "audio"},
		"audio":      map[string]any{"format": "wav", "voice": "alloy"},
		"response_format": map[string]any{
			"type": "json_object",
		},
	}); err != nil {
		t.Fatalf("SetExtension: %v", err)
	}
	return request
}

func newCoreChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Model               string            `json:"model"`
			Stream              bool              `json:"stream"`
			Messages            []json.RawMessage `json:"messages"`
			Tools               []json.RawMessage `json:"tools"`
			Modalities          []string          `json:"modalities"`
			MaxCompletionTokens int64             `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		if request.URL.Path != "/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("request identity = %q/%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		if body.Model != "gpt-5.2" || len(body.Messages) != 4 || len(body.Tools) != 1 || body.MaxCompletionTokens != 512 {
			t.Errorf("request shape = model %q messages %d tools %d max %d", body.Model, len(body.Messages), len(body.Tools), body.MaxCompletionTokens)
		}
		if strings.Join(body.Modalities, ",") != "text,audio" {
			t.Errorf("modalities = %v", body.Modalities)
		}
		var assistant struct {
			Audio struct {
				ID string `json:"id"`
			} `json:"audio"`
		}
		if err := json.Unmarshal(body.Messages[2], &assistant); err != nil || assistant.Audio.ID != "audio-prev" {
			t.Errorf("assistant audio replay = %q/%v", assistant.Audio.ID, err)
		}
		if body.Stream {
			writeCoreChatStream(writer)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, coreChatCompletionJSON)
	}))
}

func writeCoreChatStream(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	chunks := []string{
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"reasoning_content":"think "}}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"content":"hello "}}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"content":"world"}}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]}}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-stream","type":"function","function":{"name":"search","arguments":"\"lynx\""}}]}}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1770000001,"model":"gpt-5.2","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}`,
	}
	for _, chunk := range chunks {
		fmt.Fprintf(writer, "data: %s\n\n", chunk)
	}
	fmt.Fprint(writer, "data: [DONE]\n\n")
}

var coreChatCompletionJSON = `{
  "id":"chatcmpl-core",
  "object":"chat.completion",
  "created":1770000000,
  "model":"gpt-5.2",
  "service_tier":"priority",
  "choices":[
    {
      "index":0,
      "finish_reason":"tool_calls",
      "message":{
        "role":"assistant",
        "reasoning_content":"checking sources",
        "content":"I found two results.",
        "refusal":"",
        "annotations":[],
        "tool_calls":[{"id":"call-2","type":"function","function":{"name":"search","arguments":"{\"q\":\"more\"}"}}],
        "audio":{"id":"audio-1","data":"` + base64.StdEncoding.EncodeToString([]byte("audio")) + `","expires_at":1770000100,"transcript":"spoken"}
      },
      "logprobs":{"content":[],"refusal":[]}
    },
    {
      "index":1,
      "finish_reason":"content_filter",
      "message":{"role":"assistant","content":"alternate","refusal":""},
      "logprobs":{"content":[],"refusal":[]}
    }
  ],
  "usage":{
    "prompt_tokens":12,
    "completion_tokens":7,
    "total_tokens":19,
    "completion_tokens_details":{"reasoning_tokens":3},
    "prompt_tokens_details":{"cached_tokens":5}
  }
}`
