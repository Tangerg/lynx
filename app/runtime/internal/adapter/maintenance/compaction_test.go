package maintenance

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

// constClient adapts a fixed client to the per-call [ClientFunc] the
// maintenance services take — these tests don't exercise the runtime's
// utility-role swap, just a stable stub model.
func constClient(c *chatclient.Client) ClientFunc {
	return func(context.Context) *chatclient.Client { return c }
}

// TestCompactor_NopBelowThreshold doesn't talk to a real LLM —
// confirms the early-return path when there aren't enough
// messages to bother compacting.
func TestCompactor_NopBelowThreshold(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "s"
	_ = store.Write(context.Background(), sessID,
		chat.NewUserMessage(chat.NewTextPart("a")),
		chat.NewAssistantMessage(chat.NewTextPart("b")),
	)
	c := NewCompactor(store, nil /* never called */, CompactionConfig{MaxMessages: 10})
	res, err := c.MaybeCompact(context.Background(), sessID, nil)
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
	store := history.NewInMemoryStore()
	const sessID = "sess-compact"
	const total = 20
	for range total {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage(chat.NewTextPart("msg")))
	}

	client, _ := chatclient.New(newTextStubModel("BULLETS"))

	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: total, KeepRecent: 4})
	res, err := c.MaybeCompact(context.Background(), sessID, nil)
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
	if after[0].Role != chat.RoleSystem {
		t.Fatalf("first message role is %q, want system", after[0].Role)
	}
	if !strings.HasPrefix(after[0].Text(), "[Earlier conversation summary]") {
		t.Errorf("summary preamble missing, got %q", after[0].Text())
	}
}

// TestCompactor_CutBoundary is the regression for the DeepSeek 400 bug:
// when the naive cutoff (len-keepRecent) lands mid-turn — splitting an
// AssistantMessage-with-tool_calls from its ToolMessage — the compactor
// must advance the cut to the next UserMessage boundary so the kept
// `recent` slice never starts with an orphaned ToolMessage.
func TestCompactor_CutBoundary(t *testing.T) {
	store := history.NewInMemoryStore()
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

	asst := func(text string) chat.Message { return chat.NewAssistantMessage(chat.NewTextPart(text)) }
	user := func(text string) chat.Message { return chat.NewUserMessage(chat.NewTextPart(text)) }
	tool := func(id, result string) chat.Message {
		return chat.NewToolMessage(chat.ToolResult{ID: id, Name: "shell", Result: result})
	}

	msgs := []chat.Message{
		user("first question"),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "shell", Arguments: `{}`})),
		tool("c1", "ok"), // tool result — must not be orphaned at recent[0]
		asst("done"),
		user("second question"),
		asst("answer"),
	}
	for _, m := range msgs {
		_ = store.Write(context.Background(), sessID, m)
	}

	client, _ := chatclient.New(newTextStubModel("BULLETS"))
	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: 6, KeepRecent: 4})
	res, err := c.MaybeCompact(context.Background(), sessID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected compaction to fire")
	}

	after, _ := store.Read(context.Background(), sessID)
	// First message must be the system summary.
	if after[0].Role != chat.RoleSystem {
		t.Fatalf("after[0] role = %q, want system", after[0].Role)
	}
	// Second message must be the UserMessage from the second turn, never a ToolMessage.
	if after[1].Role != chat.RoleUser {
		t.Fatalf("after[1] role = %q, want user (not orphaned tool message)", after[1].Role)
	}
}

// TestCompactor_TokenTrigger fires the token-footprint trigger
// independently of message count: a conversation with far fewer messages
// than MaxMessages still compacts when one carries a large tool result,
// because byte size — not message count — is what fills a context window.
func TestCompactor_TokenTrigger(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-tokens"

	big := strings.Repeat("x", 50_000) // ~12.5k estimated tokens
	huge := chat.NewToolMessage(chat.ToolResult{ID: "c1", Name: "read", Result: big})
	_ = store.Write(context.Background(), sessID,
		chat.NewUserMessage(chat.NewTextPart("read the file")),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "read", Arguments: `{}`})),
		huge,
		chat.NewUserMessage(chat.NewTextPart("now summarize")),
	)

	client, _ := chatclient.New(newTextStubModel("BULLETS"))
	// Message bound far out of reach; token bound below the tool result —
	// so only the token trigger can fire.
	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: 1000, MaxTokens: 10_000, KeepRecent: 2})
	res, err := c.MaybeCompact(context.Background(), sessID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected the token-footprint trigger to fire on a large tool result")
	}
}

// TestCompactor_TokenTriggerShortHistory is the regression for the negative-cutoff
// panic: the token trigger fires on a conversation with FEWER messages than
// keepRecent (a couple of huge tool results). cutoff = len-keepRecent would be
// negative; MaybeCompact must skip cleanly (nothing older to summarize), not
// panic with an out-of-range index.
func TestCompactor_TokenTriggerShortHistory(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-short"

	big := strings.Repeat("x", 50_000)
	huge := chat.NewToolMessage(chat.ToolResult{ID: "c1", Name: "read", Result: big})
	// 3 messages < keepRecent (6, the production default).
	_ = store.Write(context.Background(), sessID,
		chat.NewUserMessage(chat.NewTextPart("read the file")),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "read", Arguments: `{}`})),
		huge,
	)

	client, _ := chatclient.New(newTextStubModel("BULLETS"))
	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: 1000, MaxTokens: 10_000, KeepRecent: 6})
	res, err := c.MaybeCompact(context.Background(), sessID, nil) // must not panic
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted {
		t.Fatal("expected no compaction when the whole history is within keepRecent")
	}
}

// TestCompactor_PreCompactVeto confirms a PreCompact callback returning false
// vetoes a compaction that would otherwise fire — and that it's only consulted
// once the sweep is committed (it must see a would-compact history).
func TestCompactor_PreCompactVeto(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-veto"
	const total = 20
	for range total {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage(chat.NewTextPart("msg")))
	}
	client, _ := chatclient.New(newTextStubModel("BULLETS"))
	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: total, KeepRecent: 4})

	called := false
	res, err := c.MaybeCompact(context.Background(), sessID, func(context.Context) bool {
		called = true
		return false // veto
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("preCompact should be consulted when a compaction is committed")
	}
	if res.Compacted {
		t.Fatal("a vetoing preCompact must prevent compaction")
	}
	if after, _ := store.Read(context.Background(), sessID); len(after) != total {
		t.Fatalf("history changed despite veto: len = %d, want %d", len(after), total)
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

// textStubModel is a deterministic chat.Model that returns a fixed
// reply for any prompt — enough to drive the maintenance workers'
// direct (middleware-free) LLM calls offline.
type textStubModel struct {
	reply string
}

func newTextStubModel(reply string) *textStubModel {
	return &textStubModel{reply: reply}
}

func (m *textStubModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func (m *textStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}
