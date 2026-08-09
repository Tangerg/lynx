package bootstrap

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/core/chat"
)

// replyStub is a minimal chat.Model that answers every turn with a fixed text
// reply, so tests can build a chatclient.Client without touching a provider.
type replyStub struct {
	reply string
}

func newReplyStub(reply string) *replyStub {
	return &replyStub{reply: reply}
}

func (m *replyStub) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func (m *replyStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}
