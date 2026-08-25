package storage

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

func TestMessageCodecRoundTrip(t *testing.T) {
	messages := []chat.Message{
		chat.NewSystemMessage("be concise"),
		chat.NewUserMessage(chat.NewTextPart("hello")),
	}

	codec := MessageCodec{}
	encoded, err := codec.Encode(messages)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded := make([]chat.Message, len(encoded))
	for index := range encoded {
		decoded[index], err = codec.Decode(encoded[index])
		if err != nil {
			t.Fatalf("Decode(encoded[%d]) error = %v", index, err)
		}
	}
	if !reflect.DeepEqual(decoded, messages) {
		t.Fatalf("round trip = %#v, want %#v", decoded, messages)
	}
}

func TestMessageCodecReportsInvalidMessageIndex(t *testing.T) {
	_, err := (MessageCodec{}).Encode([]chat.Message{
		chat.NewSystemMessage("valid"),
		{},
	})
	if err == nil || !strings.HasPrefix(err.Error(), "message 1") {
		t.Fatalf("Encode() error = %v, want message 1 context", err)
	}
}
