package maintenance

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
)

// TestCompactor_NopBelowThreshold doesn't talk to a real LLM —
// confirms the early-return path when there aren't enough
// messages to bother compacting.
func TestCompactor_NopBelowThreshold(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "s"
	_ = store.Write(context.Background(), sessID,
		chat.NewUserMessage("a"),
		chat.NewAssistantMessage("b"),
	)
	c := NewCompactor(store, nil /* never called */, CompactionConfig{MaxMessages: 10})
	res, err := c.MaybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted {
		t.Error("below threshold should not compact")
	}
}

// TestCompactor_Compacts drives the full path with a stub model
// that returns a fixed summary. After compaction:
//   - store size == 1 (summary) + keepRecent
//   - the surviving first message is a SystemMessage carrying
//     the [Earlier conversation summary] preamble
func TestCompactor_Compacts(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "sess-compact"
	const total = 20
	for i := 0; i < total; i++ {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage("msg"))
	}

	client, _ := chat.NewClient(newTextStubModel("BULLETS"))

	c := NewCompactor(store, client, CompactionConfig{MaxMessages: total, KeepRecent: 4})
	res, err := c.MaybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected compaction to fire")
	}
	if res.MessagesBefore != total || res.MessagesAfter != 5 {
		t.Errorf("result counts = (%d → %d), want (%d → 5)", res.MessagesBefore, res.MessagesAfter, total)
	}

	after, _ := store.Read(context.Background(), sessID)
	if len(after) != 5 {
		t.Fatalf("post-compact len = %d, want 5", len(after))
	}
	sys, ok := after[0].(*chat.SystemMessage)
	if !ok {
		t.Fatalf("first message is %T, want *chat.SystemMessage", after[0])
	}
	if !strings.HasPrefix(sys.Text, "[Earlier conversation summary]") {
		t.Errorf("summary preamble missing, got %q", sys.Text)
	}
}

// TestCompactor_CutBoundary is the regression for the DeepSeek 400 bug:
// when the naive cutoff (len-keepRecent) lands mid-turn — splitting an
// AssistantMessage-with-tool_calls from its ToolMessage — the compactor
// must advance the cut to the next UserMessage boundary so the kept
// `recent` slice never starts with an orphaned ToolMessage.
func TestCompactor_CutBoundary(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "sess-boundary"

	// Build a history that triggers the boundary bug:
	//   [0] user      <- first turn
	//   [1] assistant (with tool call — represented here as AssistantMessage)
	//   [2] tool      (result)
	//   [3] assistant (reply)
	//   [4] user      <- second turn
	//   [5] assistant
	// With MaxMessages=6, keepRecent=4 → naive cutoff = 6-4 = 2.
	// msgs[2] is a ToolMessage → recent would start orphaned.
	// The fix must advance cutoff to 4 (the UserMessage at [4]).

	asst := func(text string) chat.Message { return chat.NewAssistantMessage(text) }
	user := func(text string) chat.Message { return chat.NewUserMessage(text) }
	tool := func(id, result string) chat.Message {
		m, _ := chat.NewToolMessage([]*chat.ToolReturn{{ID: id, Name: "bash", Result: result}})
		return m
	}

	msgs := []chat.Message{
		user("first question"),
		asst(""),         // assistant turn with (notional) tool_calls
		tool("c1", "ok"), // tool result — must not be orphaned at recent[0]
		asst("done"),
		user("second question"),
		asst("answer"),
	}
	for _, m := range msgs {
		_ = store.Write(context.Background(), sessID, m)
	}

	client, _ := chat.NewClient(newTextStubModel("BULLETS"))
	c := NewCompactor(store, client, CompactionConfig{MaxMessages: 6, KeepRecent: 4})
	res, err := c.MaybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected compaction to fire")
	}

	after, _ := store.Read(context.Background(), sessID)
	// First message must be the system summary.
	if _, ok := after[0].(*chat.SystemMessage); !ok {
		t.Fatalf("after[0] = %T, want *chat.SystemMessage", after[0])
	}
	// Second message must be the UserMessage from the second turn, never a ToolMessage.
	if _, ok := after[1].(*chat.UserMessage); !ok {
		t.Fatalf("after[1] = %T, want *chat.UserMessage (not orphaned ToolMessage)", after[1])
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

// textStubModel is a deterministic chat.Model that returns a fixed
// reply for any prompt — enough to drive the maintenance workers'
// direct (middleware-free) LLM calls offline.
type textStubModel struct {
	reply    string
	defaults *chat.Options
}

func newTextStubModel(reply string) *textStubModel {
	opts, _ := chat.NewOptions("stub-maintenance")
	return &textStubModel{reply: reply, defaults: opts}
}

func (m *textStubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *textStubModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *textStubModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	return chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(m.reply),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
}

func (m *textStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}
