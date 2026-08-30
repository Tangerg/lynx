package protocol

import (
	"strings"
	"testing"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestProtocolRequestMapsCoreReasoningEffort(t *testing.T) {
	request := validReasoningRequest(t, "high")
	if err := request.Options.Extensions.Set(RequestExtensionKey, map[string]any{
		"thinkingConfig": map[string]any{"includeThoughts": true},
	}); err != nil {
		t.Fatal(err)
	}
	_, _, config, err := mapProtocolRequest("google", corechat.Options{Model: "gemini-3-pro"}, request)
	if err != nil {
		t.Fatal(err)
	}
	if config.ThinkingConfig == nil || config.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelHigh || !config.ThinkingConfig.IncludeThoughts {
		t.Fatalf("thinking config = %#v", config.ThinkingConfig)
	}
}

func TestProtocolRequestRejectsDuplicateOrUnknownReasoningEffort(t *testing.T) {
	request := validReasoningRequest(t, "high")
	if err := request.Options.Extensions.Set(RequestExtensionKey, map[string]any{
		"thinkingConfig": map[string]any{"thinkingLevel": "LOW"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := mapProtocolRequest("google", corechat.Options{Model: "gemini-3-pro"}, request); err == nil || !strings.Contains(err.Error(), "owned by options.reasoning_effort") {
		t.Fatalf("duplicate owner error = %v", err)
	}

	request = validReasoningRequest(t, "turbo")
	if _, _, _, err := mapProtocolRequest("google", corechat.Options{Model: "gemini-3-pro"}, request); err == nil || !strings.Contains(err.Error(), "unsupported value") {
		t.Fatalf("unknown effort error = %v", err)
	}
}

func validReasoningRequest(t *testing.T, effort corechat.ReasoningEffort) *corechat.Request {
	t.Helper()
	request, err := corechat.NewRequest(corechat.NewUserMessage(corechat.NewTextPart("hello")))
	if err != nil {
		t.Fatal(err)
	}
	request.Options.ReasoningEffort = effort
	return request
}
