package snapshot_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/chathistory/internal/snapshot"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestRequestDeepCopiesEveryReference(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	message := chat.NewUserMessage(chat.NewMediaPart(image))
	message.Metadata = metadata.New()
	if err := metadata.Set(message.Metadata, "turn", 1); err != nil {
		t.Fatal(err)
	}
	temperature := 0.5
	request, err := chat.NewRequest(message)
	if err != nil {
		t.Fatal(err)
	}
	request.Tools = []chat.ToolDefinition{{Name: "weather", InputSchema: []byte(`{"type":"object"}`)}}
	request.Options = chat.Options{Temperature: &temperature, Stop: []string{"END"}}
	if err := request.SetExtension("test/value", "original"); err != nil {
		t.Fatal(err)
	}

	cloned, err := snapshot.Request(request)
	if err != nil {
		t.Fatal(err)
	}
	cloned.Messages[0].Metadata["turn"][0] = '9'
	cloned.Messages[0].Parts[0].Media.Source.Bytes[0] = 9
	cloned.Tools[0].InputSchema[0] = '['
	*cloned.Options.Temperature = 1.5
	cloned.Options.Stop[0] = "MUTATED"
	cloned.Extensions["test/value"][1] = 'X'

	if string(request.Messages[0].Metadata["turn"]) != "1" || request.Messages[0].Parts[0].Media.Source.Bytes[0] != 1 || request.Tools[0].InputSchema[0] != '{' || *request.Options.Temperature != 0.5 || request.Options.Stop[0] != "END" || string(request.Extensions["test/value"]) != `"original"` {
		t.Fatalf("source request was mutated: %#v", request)
	}
}

func TestRequestAndMessagesRejectInvalidValues(t *testing.T) {
	if _, err := snapshot.Request(nil); !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("Request(nil) error = %v", err)
	}
	if _, err := snapshot.Request(&chat.Request{}); !errors.Is(err, chat.ErrInvalidRequest) {
		t.Fatalf("Request(empty) error = %v", err)
	}
	if _, err := snapshot.Messages([]chat.Message{{}}); !errors.Is(err, chat.ErrInvalidMessage) {
		t.Fatalf("Messages(invalid) error = %v", err)
	}
}

func TestMessageCopiesReasoningAndToolPointers(t *testing.T) {
	message := chat.NewAssistantMessage(
		chat.NewReasoningPart("think", []byte{1, 2}),
		chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "weather"}),
	)
	cloned := snapshot.Message(message)
	cloned.Parts[0].Signature[0] = 9
	cloned.Parts[1].ToolCall.Name = "mutated"
	if message.Parts[0].Signature[0] != 1 || message.Parts[1].ToolCall.Name != "weather" {
		t.Fatalf("source message was mutated: %#v", message)
	}

	tool := chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "weather", Result: "sunny"})
	cloned = snapshot.Message(tool)
	cloned.Parts[0].ToolResult.Result = "mutated"
	if tool.Parts[0].ToolResult.Result != "sunny" {
		t.Fatalf("source tool result was mutated: %#v", tool)
	}
}
