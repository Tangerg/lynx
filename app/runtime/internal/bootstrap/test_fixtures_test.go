package bootstrap

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/core/model/chat"
)

// replyStub is a minimal chat.Model that answers every turn with a fixed text
// reply, so tests can build a chat.Client without touching a provider.
type replyStub struct {
	reply    string
	defaults *chat.Options
}

func newReplyStub(reply string) *replyStub {
	opts, _ := chat.NewOptions("stub")
	return &replyStub{reply: reply, defaults: opts}
}

func (m *replyStub) DefaultOptions() chat.Options { return *m.defaults }

func (m *replyStub) Metadata() chat.ModelMetadata {
	return chat.ModelMetadata{Provider: "stub"}
}

func (m *replyStub) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	return chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(m.reply),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
}

func (m *replyStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}
