package agent_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/core/chat"
)

type callOnlyModel struct{}

func (callOnlyModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, nil
}

type callStreamModel struct{ callOnlyModel }

func (callStreamModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(func(*chat.Response, error) bool) {}
}

func TestChatEnablesStreamingOnlyWhenSupported(t *testing.T) {
	callOnly := agent.Chat(callOnlyModel{})
	if callOnly.Model == nil {
		t.Fatal("Chat left Model unset")
	}
	if callOnly.Streamer != nil {
		t.Fatal("Chat set a Streamer for a call-only model")
	}

	both := agent.Chat(callStreamModel{})
	if both.Model == nil || both.Streamer == nil {
		t.Fatalf("Chat did not enable streaming for a streaming model: %+v", both)
	}
}

func TestRequireTypeProducesDistinctDefaultBindingKeys(t *testing.T) {
	type topic struct{ Title string }
	type research struct{ Notes string }

	topicKey := agent.RequireType[topic]()
	researchKey := agent.RequireType[research]()

	if !strings.HasPrefix(topicKey, agent.DefaultBindingName+":") {
		t.Fatalf("RequireType key %q does not target the default binding %q", topicKey, agent.DefaultBindingName)
	}
	if topicKey == researchKey {
		t.Fatalf("RequireType collapsed distinct types to one key %q", topicKey)
	}
}
