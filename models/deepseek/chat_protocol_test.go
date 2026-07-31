package deepseek_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/deepseek"
)

func TestOpenAIChat_ReasoningReplay(t *testing.T) {
	tests := []struct {
		name              string
		messages          []corechat.Message
		wantReasoningWire bool
	}{
		{
			name: "ordinary previous turn omits reasoning",
			messages: []corechat.Message{
				corechat.NewUserMessage(corechat.NewTextPart("first")),
				corechat.NewAssistantMessage(
					corechat.NewReasoningPart("private chain", nil),
					corechat.NewTextPart("first answer"),
				),
				corechat.NewUserMessage(corechat.NewTextPart("second")),
			},
		},
		{
			name: "tool turn replays reasoning",
			messages: []corechat.Message{
				corechat.NewUserMessage(corechat.NewTextPart("search")),
				corechat.NewAssistantMessage(
					corechat.NewReasoningPart("need a tool", nil),
					corechat.NewToolCallPart(corechat.ToolCall{ID: "call-1", Name: "search", Arguments: `{"q":"lynx"}`}),
				),
				corechat.NewToolMessage(corechat.ToolResult{ID: "call-1", Name: "search", Result: "found"}),
			},
			wantReasoningWire: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var wireRequest struct {
				Messages []map[string]any `json:"messages"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if err := json.NewDecoder(request.Body).Decode(&wireRequest); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"id":"chat-1","model":"deepseek-v4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
			t.Cleanup(server.Close)

			model, err := deepseek.NewOpenAIChat(deepseek.OpenAIChatConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
				DefaultOptions: corechat.Options{
					Model: "deepseek-v4",
				},
			})
			if err != nil {
				t.Fatalf("NewOpenAIChat: %v", err)
			}
			if _, err := model.Call(t.Context(), &corechat.Request{Messages: test.messages}); err != nil {
				t.Fatalf("Call: %v", err)
			}

			assistant := findAssistantMessage(t, wireRequest.Messages)
			_, hasReasoning := assistant["reasoning_content"]
			if hasReasoning != test.wantReasoningWire {
				t.Fatalf("reasoning_content present = %v; want %v; message = %#v", hasReasoning, test.wantReasoningWire, assistant)
			}
		})
	}
}

func findAssistantMessage(t *testing.T, messages []map[string]any) map[string]any {
	t.Helper()
	for _, message := range messages {
		if message["role"] == "assistant" {
			return message
		}
	}
	t.Fatal("assistant message not found")
	return nil
}
