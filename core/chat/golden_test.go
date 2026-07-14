package chat_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestRequestGolden(t *testing.T) {
	attachment, err := media.NewURI("image/png", "https://example.com/lynx.png")
	if err != nil {
		t.Fatal(err)
	}

	system := chat.NewSystemMessage("Answer precisely.")
	user := chat.NewUserMessage(chat.NewTextPart("What is shown?"), chat.NewMediaPart(attachment))
	assistant := chat.NewAssistantMessage(
		chat.NewReasoningPart("I should inspect the image.", []byte("opaque-signature")),
		chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "inspect_image", Arguments: `{"detail":"high"}`}),
	)
	tool := chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "inspect_image", Result: "A lynx."})
	request, err := chat.NewRequest(system, user, assistant, tool)
	if err != nil {
		t.Fatal(err)
	}
	request.Tools = []chat.ToolDefinition{{
		Name:        "inspect_image",
		Description: "Inspect an image",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"detail":{"type":"string"}}}`),
	}}
	request.Options = chat.Options{Model: "provider-model", Temperature: pointer(0.2), MaxTokens: pointer(int64(256))}
	if err := request.SetExtension("openai/response_format", map[string]string{"type": "text"}); err != nil {
		t.Fatal(err)
	}
	assertChatGolden(t, "request.golden.json", request)
}

func TestResponseGolden(t *testing.T) {
	firstMessage := chat.NewAssistantMessage(chat.NewTextPart("A lynx."))
	secondMessage := chat.NewAssistantMessage(chat.NewTextPart("A wild cat."))
	response, err := chat.NewResponse(
		chat.Choice{Index: 0, Message: &firstMessage, FinishReason: chat.FinishReasonStop},
		chat.Choice{Index: 1, Message: &secondMessage, FinishReason: chat.FinishReasonLength},
	)
	if err != nil {
		t.Fatal(err)
	}
	response.ID = "response-1"
	response.Model = "provider-model"
	reasoning := int64(4)
	cacheRead := int64(8)
	response.Usage = chat.Usage{InputTokens: 32, OutputTokens: 12, ReasoningTokens: &reasoning, CacheReadInputTokens: &cacheRead}
	if err := response.SetExtension("openai/system_fingerprint", "fp-1"); err != nil {
		t.Fatal(err)
	}
	if err := response.Choices[0].SetExtension("openai/logprobs", []float64{-0.1, -0.2}); err != nil {
		t.Fatal(err)
	}
	assertChatGolden(t, "response.golden.json", response)
}

func TestGoldenMetadataIsJSONSafe(t *testing.T) {
	value := metadata.New()
	if err := metadata.Set(value, "fixture", "chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(value); err != nil {
		t.Fatal(err)
	}
}

func assertChatGolden(t *testing.T, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
