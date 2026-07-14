package chat_test

import (
	"context"
	"iter"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

type streamOnlyModel struct{}

func (streamOnlyModel) Stream(_ context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		if err := request.Validate(); err != nil {
			yield(nil, err)
			return
		}
		yield(&chat.Response{}, nil)
	}
}

var _ chat.Streamer = streamOnlyModel{}

func TestStreamOnlyProviderSatisfiesStreamer(t *testing.T) {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	count := 0
	for response, err := range (streamOnlyModel{}).Stream(context.Background(), request) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if response == nil {
			t.Fatal("Stream yielded nil response")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("Stream yield count = %d, want 1", count)
	}
}

func TestStreamerPublicShape(t *testing.T) {
	streamerType := reflect.TypeFor[chat.Streamer]()
	if streamerType.NumMethod() != 1 {
		t.Fatalf("Streamer method count = %d, want 1", streamerType.NumMethod())
	}
	method := streamerType.Method(0)
	if method.Name != "Stream" {
		t.Fatalf("Streamer method = %q, want Stream", method.Name)
	}

	streamType := method.Type
	wantSequence := reflect.TypeFor[iter.Seq2[*chat.Response, error]]()
	if streamType.NumIn() != 2 || streamType.In(0) != reflect.TypeFor[context.Context]() || streamType.In(1) != reflect.TypeFor[*chat.Request]() {
		t.Fatalf("Stream inputs = %v, want (context.Context, *chat.Request)", streamType)
	}
	if streamType.NumOut() != 1 || streamType.Out(0) != wantSequence {
		t.Fatalf("Stream output = %v, want %v", streamType, wantSequence)
	}
}
