package anthropic

import (
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestNativeClaudeSamplingContract(t *testing.T) {
	dialect := Dialect{Provider: "anthropic", MaxTemperature: 1, RejectTopK: true, RejectTopP: true}
	tests := []struct {
		name    string
		options corechat.Options
		want    string
	}{
		{name: "top k", options: corechat.Options{TopK: new(int64(10))}, want: "top_k is not supported"},
		{name: "top p", options: corechat.Options{TopP: new(0.9)}, want: "top_p is not supported"},
		{name: "temperature", options: corechat.Options{Temperature: new(1.1)}, want: "between 0 and 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validNativeRequest(t)
			request.Options = tt.options
			_, err := mapProtocolRequest(corechat.Options{Model: "claude-opus-4-6"}, request, dialect)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestNativeClaudeRejectsZeroMaxTokens(t *testing.T) {
	request := validNativeRequest(t)
	request.Options.MaxTokens = new(int64(0))
	_, err := mapProtocolRequest(corechat.Options{Model: "claude-opus-4-6"}, request, Dialect{Provider: "anthropic"})
	if err == nil || !strings.Contains(err.Error(), "max_tokens must be greater than zero") {
		t.Fatalf("error = %v, want max_tokens validation failure", err)
	}
}

func TestNativeClaudePreservesServerToolResponse(t *testing.T) {
	message := &anthropicsdk.Message{
		ID:    "msg-server-tool",
		Model: "claude-opus-4-6",
		Content: []anthropicsdk.ContentBlockUnion{
			{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: []byte(`{"query":"official docs"}`)},
			{Type: "text", Text: "result"},
		},
		StopReason: anthropicsdk.StopReasonEndTurn,
	}
	response, err := mapProtocolMessage(message, "anthropic")
	if err != nil {
		t.Fatalf("mapProtocolMessage: %v", err)
	}
	if response.Output.Message == nil || response.Output.Message.Text() != "result" {
		t.Fatalf("Core response = %#v", response.Output)
	}
	preserved, found, err := response.Metadata.Extra.Decode[anthropicsdk.Message](ResponseExtensionKey)
	if err != nil || !found {
		t.Fatalf("decode native response = found %v, error %v", found, err)
	}
	if len(preserved.Content) != 2 || preserved.Content[0].Type != "server_tool_use" || preserved.Content[0].ID != "srvtoolu_1" {
		t.Fatalf("preserved content = %#v", preserved.Content)
	}
}

func TestReasoningReplayIsScopedToIssuingProvider(t *testing.T) {
	message := &anthropicsdk.Message{
		ID:         "msg-provider-scope",
		Model:      "compatible-model",
		Content:    []anthropicsdk.ContentBlockUnion{{Type: "thinking", Thinking: "private", Signature: "provider-signature"}},
		StopReason: anthropicsdk.StopReasonEndTurn,
	}
	response, err := mapProtocolMessage(message, "minimax")
	if err != nil {
		t.Fatalf("mapProtocolMessage: %v", err)
	}
	assistant := *response.Output.Message

	matching, err := mapProtocolAssistant(assistant, "minimax")
	if err != nil || len(matching) != 1 || matching[0].GetSignature() == nil || *matching[0].GetSignature() != "provider-signature" {
		t.Fatalf("matching replay = %#v, error %v", matching, err)
	}
	foreign, err := mapProtocolAssistant(assistant, "anthropic")
	if err != nil {
		t.Fatalf("foreign replay: %v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("foreign provider received opaque reasoning: %#v", foreign)
	}
}

func validNativeRequest(t *testing.T) *corechat.Request {
	t.Helper()
	request, err := corechat.NewRequest(corechat.NewUserMessage(corechat.NewTextPart("hello")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return request
}
