package openai_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corechat "github.com/Tangerg/scope/core/chat"
	scopeopenai "github.com/Tangerg/scope/models/protocol/openai"
)

func TestTextReasoningDialects(t *testing.T) {
	tests := []struct {
		name            string
		dialect         scopeopenai.Dialect
		responseField   string
		wantReplayField string
		withToolCall    bool
	}{
		{name: "reasoning_content output only", dialect: scopeopenai.ReasoningContentDialect("test"), responseField: "reasoning_content"},
		{name: "reasoning_content full replay", dialect: scopeopenai.ReasoningContentReplayDialect("test"), responseField: "reasoning_content", wantReplayField: "reasoning_content"},
		{name: "reasoning_content tool replay", dialect: scopeopenai.ReasoningContentToolReplayDialect("test"), responseField: "reasoning_content", wantReplayField: "reasoning_content", withToolCall: true},
		{name: "reasoning output only", dialect: scopeopenai.ReasoningDialect("test"), responseField: "reasoning"},
		{name: "reasoning full replay", dialect: scopeopenai.ReasoningReplayDialect("test"), responseField: "reasoning", wantReplayField: "reasoning"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestBody struct {
				Messages []map[string]any `json:"messages"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				response := map[string]any{
					"id":    "chat-1",
					"model": "provider-model",
					"choices": []any{map[string]any{
						"index":         0,
						"finish_reason": "stop",
						"message": map[string]any{
							"role":             "assistant",
							"content":          "answer",
							test.responseField: "fresh reasoning",
						},
					}},
				}
				if err := json.NewEncoder(writer).Encode(response); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			t.Cleanup(server.Close)

			adapter, err := scopeopenai.NewCompatibleChat(scopeopenai.ChatConfig{
				APIKey:         "test-key",
				DefaultOptions: corechat.Options{Model: "provider-model"},
				BaseURL:        server.URL,
			}, test.dialect)
			if err != nil {
				t.Fatalf("NewCompatibleChat: %v", err)
			}
			field := scopeopenai.TextReasoningContent
			if test.responseField == "reasoning" {
				field = scopeopenai.TextReasoning
			}
			priorReasoning, err := scopeopenai.NewTextReasoningPart("test", field, "prior reasoning")
			if err != nil {
				t.Fatalf("NewTextReasoningPart: %v", err)
			}
			assistantParts := []corechat.Part{
				priorReasoning,
				corechat.NewTextPart("prior answer"),
			}
			if test.withToolCall {
				assistantParts = append(assistantParts, corechat.NewToolCallPart(corechat.ToolCall{
					ID: "call-1", Name: "lookup", Arguments: `{}`,
				}))
			}
			response, err := adapter.Call(t.Context(), &corechat.Request{Messages: []corechat.Message{
				corechat.NewUserMessage(corechat.NewTextPart("first")),
				corechat.NewAssistantMessage(assistantParts...),
				corechat.NewUserMessage(corechat.NewTextPart("second")),
			}})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}

			assistant := requestBody.Messages[1]
			for _, field := range []string{"reasoning", "reasoning_content"} {
				value, found := assistant[field]
				if field == test.wantReplayField {
					if !found || value != "prior reasoning" {
						t.Errorf("request %s = %#v/%v; want prior reasoning", field, value, found)
					}
				} else if found {
					t.Errorf("unexpected request field %s = %#v", field, value)
				}
			}
			parts := response.Output.Message.Parts
			if len(parts) != 2 || parts[0].Kind != corechat.PartReasoning || parts[0].Text != "fresh reasoning" || parts[1].Text != "answer" {
				t.Fatalf("response parts = %#v", parts)
			}
		})
	}
}

func TestChatTokenLimitFieldMatchesProtocol(t *testing.T) {
	tests := []struct {
		name      string
		construct func(scopeopenai.ChatConfig) (*scopeopenai.Chat, error)
		wantField string
	}{
		{name: "native", construct: scopeopenai.NewChat, wantField: "max_completion_tokens"},
		{
			name: "compatible",
			construct: func(config scopeopenai.ChatConfig) (*scopeopenai.Chat, error) {
				return scopeopenai.NewCompatibleChat(config, scopeopenai.Dialect{Provider: "test", TokenLimitField: scopeopenai.TokenLimitMaxTokens})
			},
			wantField: "max_tokens",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"id":"chat-1","model":"model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`))
			}))
			t.Cleanup(server.Close)

			maxTokens := int64(321)
			adapter, err := test.construct(scopeopenai.ChatConfig{
				APIKey: "test-key",
				DefaultOptions: corechat.Options{
					Model:     "model",
					MaxTokens: &maxTokens,
				},
				BaseURL: server.URL,
			})
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			if _, err := adapter.Call(t.Context(), &corechat.Request{Messages: []corechat.Message{
				corechat.NewUserMessage(corechat.NewTextPart("hello")),
			}}); err != nil {
				t.Fatalf("Call: %v", err)
			}
			if body[test.wantField] != float64(maxTokens) {
				t.Fatalf("%s = %#v; want %d", test.wantField, body[test.wantField], maxTokens)
			}
			otherField := "max_tokens"
			if test.wantField == otherField {
				otherField = "max_completion_tokens"
			}
			if _, found := body[otherField]; found {
				t.Fatalf("unexpected %s in request: %#v", otherField, body)
			}
		})
	}
}

func TestDialectRequiresAnExplicitTokenLimitField(t *testing.T) {
	dialect := scopeopenai.Dialect{Provider: "test"}
	if err := dialect.Validate(); err == nil {
		t.Fatal("Dialect.Validate accepted an implicit token limit field")
	}
	if scopeopenai.TokenLimitField("").Valid() {
		t.Fatal("zero TokenLimitField is valid")
	}
}
