package conformance_test

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/internal/conformance"
)

type scriptedChat struct{}

func (scriptedChat) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return &chat.Response{ID: "call"}, nil
}

func (scriptedChat) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		yield(&chat.Response{ID: "stream"}, nil)
	}
}

var (
	_ chat.Model    = scriptedChat{}
	_ chat.Streamer = scriptedChat{}
)

func TestChatSuite(t *testing.T) {
	callAsserted := false
	streamAsserted := false
	conformance.ChatSuite{
		New: func(*testing.T) (chat.Model, chat.Streamer) {
			model := scriptedChat{}
			return model, model
		},
		Request: func(t *testing.T) *chat.Request {
			request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			return request
		},
		AssertCall: func(t *testing.T, response *chat.Response) {
			callAsserted = true
			if response.ID != "call" {
				t.Fatalf("Call response ID = %q", response.ID)
			}
		},
		AssertStream: func(t *testing.T, responses []*chat.Response) {
			streamAsserted = true
			if len(responses) != 1 || responses[0].ID != "stream" {
				t.Fatalf("Stream responses = %#v", responses)
			}
		},
	}.Run(t)

	if !callAsserted || !streamAsserted {
		t.Fatalf("assert callbacks = call:%v stream:%v", callAsserted, streamAsserted)
	}
}
