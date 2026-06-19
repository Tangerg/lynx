package runtime

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/core/model/chat"
)

// replyStub is a minimal chat.Model that answers every turn with a fixed
// text reply (no tool calls), so a chat turn completes in one round.
type replyStub struct {
	reply    string
	defaults *chat.Options
}

func newReplyStub(reply string) *replyStub {
	opts, _ := chat.NewOptions("stub-a2a")
	return &replyStub{reply: reply, defaults: opts}
}

func (m *replyStub) DefaultOptions() chat.Options { return *m.defaults }
func (m *replyStub) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

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

// TestA2AAgent_RunYieldsReply proves the A2A server adapter maps an inbound
// message onto a one-shot engine turn and yields the assistant's reply as a
// single chunk — the bridge a2a.NewExecutor drives onto the task lifecycle.
func TestA2AAgent_RunYieldsReply(t *testing.T) {
	client, err := chat.NewClient(newReplyStub("done: built the thing"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	eng, err := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	agent := a2aAgent{engine: eng}

	var chunks []string
	for chunk, err := range agent.Run(context.Background(), "build the thing") {
		if err != nil {
			t.Fatalf("Run yielded error: %v", err)
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (one-shot)", len(chunks))
	}
	if chunks[0] != "done: built the thing" {
		t.Errorf("reply = %q, want the assistant's text", chunks[0])
	}
}
