package codec

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

func TestCurrentWireRoundTripsEveryRole(t *testing.T) {
	image, err := media.NewURI("image/png", "https://example.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	messages := []chat.Message{
		chat.NewSystemMessage("rules"),
		chat.NewUserMessage(chat.NewTextPart("hello"), chat.NewMediaPart(image)),
		chat.NewAssistantMessage(
			chat.NewReasoningPart("thinking", []byte{1, 2}),
			chat.NewTextPart("answer"),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "weather", Arguments: `{}`}),
		),
		chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "weather", Result: "sunny", IsError: true}),
	}
	for _, message := range messages {
		message := message
		t.Run(string(message.Role), func(t *testing.T) {
			raw, err := EncodeMessage(message)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte(`"role":"`+string(message.Role)+`"`)) || bytes.Contains(raw, []byte(`"type":`)) {
				t.Fatalf("unexpected current wire: %s", raw)
			}
			decoded, err := DecodeMessage(raw)
			if err != nil {
				t.Fatal(err)
			}
			roundTrip, err := EncodeMessage(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(roundTrip, raw) {
				t.Fatalf("wire did not reach a fixed point\n got: %s\nwant: %s", roundTrip, raw)
			}
		})
	}
}

func TestCodecRejectsInvalidCurrentAndHistoricalWire(t *testing.T) {
	if _, err := EncodeMessage(chat.Message{}); !errors.Is(err, chat.ErrInvalidMessage) {
		t.Fatalf("invalid message error = %v", err)
	}
	for _, raw := range []string{
		`{}`,
		`{"role":"future","parts":[{"kind":"text","text":"hello"}]}`,
		`{"role":"user","parts":[{"kind":"future"}]}`,
		`{"type":"user","text":"legacy"}`,
	} {
		if _, err := DecodeMessage([]byte(raw)); !errors.Is(err, chat.ErrInvalidMessage) {
			t.Fatalf("DecodeMessage(%s) error = %v", raw, err)
		}
	}
}
