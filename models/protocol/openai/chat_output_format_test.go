package openai

import (
	"encoding/json"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestOutputFormatMapsToChatAndResponsesNativeShapes(t *testing.T) {
	format, err := corechat.NewJSONSchemaOutputFormat(
		"answer",
		json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer"}}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	format.Description = "Answer payload"

	chatFormat, err := mapChatOutputFormat(&format)
	if err != nil {
		t.Fatal(err)
	}
	if chatFormat.OfJSONSchema == nil || chatFormat.OfJSONSchema.JSONSchema.Name != "answer" {
		t.Fatalf("Chat response_format = %#v", chatFormat)
	}

	responsesFormat, err := mapResponsesOutputFormat(&format)
	if err != nil {
		t.Fatal(err)
	}
	if responsesFormat.OfJSONSchema == nil || responsesFormat.OfJSONSchema.Name != "answer" {
		t.Fatalf("Responses text.format = %#v", responsesFormat)
	}
}

func TestResponsesOutputFormatRejectsDuplicateExtensionSource(t *testing.T) {
	extensions := metadata.Map{}
	if err := extensions.Set(ResponsesRequestExtensionKey, map[string]any{
		"text": map[string]any{
			"verbosity": "low",
			"format":    map[string]any{"type": "json_object"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rejectResponsesOutputFormatExtension(extensions); err == nil {
		t.Fatal("text.format extension unexpectedly accepted")
	}
}

func TestOutputFormatFallsBackOnlyWhenDialectLacksNativeShape(t *testing.T) {
	format, err := corechat.NewJSONSchemaOutputFormat("answer", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	params := openaisdk.ChatCompletionNewParams{
		Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("question")},
	}
	dialect := Dialect{NativeOutputFormat: func(formatType corechat.OutputFormatType) bool {
		return formatType != corechat.OutputFormatJSONSchema
	}}
	if applyChatOutputFormatErr := applyChatOutputFormat(&format, &params, dialect); applyChatOutputFormatErr != nil {
		t.Fatal(applyChatOutputFormatErr)
	}
	if params.ResponseFormat.OfJSONSchema != nil || len(params.Messages) != 2 {
		t.Fatalf("fallback request = %#v", params)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat json.RawMessage `json:"response_format"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Messages[0].Role != "system" || !strings.Contains(wire.Messages[0].Content, `{"type":"object"}`) || len(wire.ResponseFormat) != 0 {
		t.Fatalf("fallback wire request = %s", encoded)
	}
}
