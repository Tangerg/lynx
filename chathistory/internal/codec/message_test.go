package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
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

func TestDecodeLegacyWireEveryRole(t *testing.T) {
	fixtures := []struct {
		name string
		raw  string
		want chat.Message
	}{
		{name: "system", raw: `{"type":"system","text":"rules","metadata":{"source":"legacy"}}`, want: messageWithMetadata(t, chat.NewSystemMessage("rules"))},
		{name: "user text", raw: `{"type":"user","text":"hello","metadata":{"source":"legacy"}}`, want: messageWithMetadata(t, chat.NewUserMessage(chat.NewTextPart("hello")))},
		{name: "user media", raw: `{"type":"user","media":[{"mime":"image/png","source":{"kind":"uri","uri":"https://example.com/image.png"}}],"metadata":{"source":"legacy"}}`, want: messageWithMetadata(t, legacyMediaMessage(t))},
		{name: "assistant", raw: `{"type":"assistant","parts":[{"kind":"reasoning","text":"think","signature":"AQI="},{"kind":"text","text":"answer"},{"kind":"tool_call","id":"call-1","name":"weather","arguments":"{}"}],"metadata":{"source":"legacy"}}`, want: messageWithMetadata(t, chat.NewAssistantMessage(
			chat.NewReasoningPart("think", []byte{1, 2}),
			chat.NewTextPart("answer"),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "weather", Arguments: `{}`}),
		))},
		{name: "tool", raw: `{"type":"tool","tool_returns":[{"id":"call-1","name":"weather","result":"sunny"}],"metadata":{"source":"legacy"}}`, want: messageWithMetadata(t, chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "weather", Result: "sunny"}))},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			got, err := DecodeMessage([]byte(fixture.raw))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, fixture.want) {
				t.Fatalf("decoded\n got: %#v\nwant: %#v", got, fixture.want)
			}
		})
	}
}

func TestCodecRejectsInvalidAndAmbiguousWire(t *testing.T) {
	if _, err := EncodeMessage(chat.Message{}); err == nil {
		t.Fatal("invalid message encoded")
	}
	tests := []struct {
		raw    string
		target error
	}{
		{raw: `{}`, target: ErrUnknownWire},
		{raw: `{"role":"user","type":"user","parts":[{"kind":"text","text":"hello"}]}`, target: ErrAmbiguousWire},
		{raw: `{"type":"future"}`, target: ErrUnknownWire},
		{raw: `{"type":"assistant","parts":[{"kind":"future"}]}`, target: ErrUnknownWire},
	}
	for _, test := range tests {
		if _, err := DecodeMessage([]byte(test.raw)); !errors.Is(err, test.target) {
			t.Fatalf("DecodeMessage(%s) error = %v", test.raw, err)
		}
	}
}

func messageWithMetadata(t *testing.T, message chat.Message) chat.Message {
	t.Helper()
	message.Metadata = metadata.New()
	if err := metadata.Set(message.Metadata, "source", "legacy"); err != nil {
		t.Fatal(err)
	}
	return message
}

func legacyMediaMessage(t *testing.T) chat.Message {
	t.Helper()
	var image media.Media
	if err := json.Unmarshal([]byte(`{"mime":"image/png","source":{"kind":"uri","uri":"https://example.com/image.png"}}`), &image); err != nil {
		t.Fatal(err)
	}
	return chat.NewUserMessage(chat.NewMediaPart(&image))
}
