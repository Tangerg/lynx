package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
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
	res, err := c.MaybeCompact(context.Background(), sessID, 0, nil)
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
	res, err := c.MaybeCompact(context.Background(), sessID, 0, nil)
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
	res, err := c.MaybeCompact(context.Background(), sessID, 0, nil)
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

// TestCompactor_PreservesToolPairsAcrossCutoffs is the general invariant lock
// behind the specific DeepSeek 400 regression ([TestCompactor_CutBoundary]):
// no matter where the naive cutoff lands, a surviving tool-result must keep its
// originating tool-call and vice versa. It drives one interleaved history —
// parallel tool calls, multi-result tool messages — through every keepRecent so
// the cutoff sweeps every message boundary, and asserts the pairing invariant
// on the post-compaction history each time. A cutoff can only ever split a pair
// if it falls between an assistant's tool-call and its tool-result, and the
// user-boundary advance makes that impossible; this pins that guarantee.
func TestCompactor_PreservesToolPairsAcrossCutoffs(t *testing.T) {
	asst := func(text string) chat.Message { return chat.NewAssistantMessage(chat.NewTextPart(text)) }
	user := func(text string) chat.Message { return chat.NewUserMessage(chat.NewTextPart(text)) }
	call := func(ids ...string) chat.Message {
		parts := make([]chat.Part, len(ids))
		for i, id := range ids {
			parts[i] = chat.NewToolCallPart(chat.ToolCall{ID: id, Name: "shell", Arguments: `{}`})
		}
		return chat.NewAssistantMessage(parts...)
	}
	result := func(ids ...string) chat.Message {
		results := make([]chat.ToolResult, len(ids))
		for i, id := range ids {
			results[i] = chat.ToolResult{ID: id, Name: "shell", Result: "ok"}
		}
		return chat.NewToolMessage(results...)
	}

	// Complete turns only (each tool-call answered) — an interleave of a single
	// call, parallel calls in one assistant message, and a multi-result tool
	// message, so the invariant is exercised, not just a lone pair.
	template := []chat.Message{
		user("q1"),
		call("c1"), result("c1"), asst("a1"),
		user("q2"),
		call("c2", "c3"), result("c2", "c3"), asst("a2"), // parallel calls, one tool message
		user("q3"),
		call("c4"), result("c4"), asst("a3"),
	}

	for keepRecent := 1; keepRecent < len(template); keepRecent++ {
		t.Run(fmt.Sprintf("keepRecent=%d", keepRecent), func(t *testing.T) {
			store := history.NewInMemoryStore()
			const sessID = "sess-pairs"
			for _, m := range template {
				_ = store.Write(t.Context(), sessID, m)
			}
			client, _ := chatclient.New(newTextStubModel("BULLETS"))
			// MaxMessages == len forces the count trigger every run so the cutoff
			// logic actually executes for each keepRecent.
			c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: len(template), KeepRecent: keepRecent})
			if _, err := c.MaybeCompact(t.Context(), sessID, 0, nil); err != nil {
				t.Fatal(err)
			}
			after, _ := store.Read(t.Context(), sessID)
			assertNoOrphanToolParts(t, after)
		})
	}
}

// assertNoOrphanToolParts fails if the surviving history contains a tool-result
// whose originating tool-call was dropped, or a tool-call whose result was
// dropped — the pairing a strict provider rejects with 400. Compaction folds
// the older slice into a summary (no tool parts), so every surviving pair must
// come from the verbatim `recent` slice intact.
func assertNoOrphanToolParts(t *testing.T, msgs []chat.Message) {
	t.Helper()
	calls, results := map[string]bool{}, map[string]bool{}
	for _, m := range msgs {
		for _, p := range m.Parts {
			switch p.Kind {
			case chat.PartToolCall:
				if p.ToolCall != nil {
					calls[p.ToolCall.ID] = true
				}
			case chat.PartToolResult:
				if p.ToolResult != nil {
					results[p.ToolResult.ID] = true
				}
			}
		}
	}
	for id := range results {
		if !calls[id] {
			t.Errorf("orphan tool-result %q: its tool-call did not survive compaction", id)
		}
	}
	for id := range calls {
		if !results[id] {
			t.Errorf("dangling tool-call %q: its tool-result did not survive compaction", id)
		}
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
	res, err := c.MaybeCompact(context.Background(), sessID, 0, nil)
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
	res, err := c.MaybeCompact(context.Background(), sessID, 0, nil) // must not panic
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
	res, err := c.MaybeCompact(context.Background(), sessID, 0, func(context.Context) bool {
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

// TestCompactor_LadderTrimsUnderBudgetSkippingLLM drives the deterministic rung:
// a conversation over the token budget purely because of one large OLD tool
// result. Trimming that result to a preview brings it under budget, so the
// summary is skipped — no LLM call, no message dropped, no boundary reported —
// and the store keeps every message with the old body previewed.
func TestCompactor_LadderTrimsUnderBudgetSkippingLLM(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-ladder-trim"
	big := strings.Repeat("y", 20_000)
	_ = store.Write(t.Context(), sessID,
		chat.NewUserMessage(chat.NewTextPart("q1")),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "read", Arguments: `{}`})),
		chat.NewToolMessage(chat.ToolResult{ID: "c1", Name: "read", Result: big}), // old + large
		chat.NewAssistantMessage(chat.NewTextPart("a1")),
		chat.NewUserMessage(chat.NewTextPart("q2")),
		chat.NewAssistantMessage(chat.NewTextPart("done")), // recent
	)

	model := newTextStubModel("SUMMARY")
	client, _ := chatclient.New(model)
	// Count trigger far out of reach; token trigger below the big result — so
	// only the token trigger fires, and the deterministic trim can clear it.
	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: 1000, MaxTokens: 4000, KeepRecent: 2})

	res, err := c.MaybeCompact(t.Context(), sessID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted {
		t.Fatal("a deterministic trim must not report a compaction boundary")
	}
	if model.calls != 0 {
		t.Fatalf("LLM Call fired %d times; the trim rung must skip the summary", model.calls)
	}

	after, _ := store.Read(t.Context(), sessID)
	if len(after) != 6 {
		t.Fatalf("post-trim len = %d, want 6 (no messages dropped)", len(after))
	}
	if after[0].Role != chat.RoleUser {
		t.Fatalf("after[0] role = %q, want user (no summary prepended)", after[0].Role)
	}
	trimmedResult := after[2].Parts[0].ToolResult.Result
	if len(trimmedResult) >= len(big) || !strings.Contains(trimmedResult, "trimmed on compaction") {
		t.Fatalf("the old tool result was not previewed: len %d, body %.60q", len(trimmedResult), trimmedResult)
	}
	if after[5].Text() != "done" {
		t.Fatalf("recent message was altered: %q", after[5].Text())
	}
	assertNoOrphanToolParts(t, after)
}

// TestCompactor_LadderStillOverGoesToLLM confirms the trim rung is a rung, not a
// replacement: when trimming can't clear the trigger (here the message-count
// trigger, which a body trim never reduces) the compactor still falls through to
// the LLM summary.
func TestCompactor_LadderStillOverGoesToLLM(t *testing.T) {
	store := history.NewInMemoryStore()
	const sessID = "sess-ladder-llm"
	big := strings.Repeat("y", 20_000)
	_ = store.Write(t.Context(), sessID,
		chat.NewUserMessage(chat.NewTextPart("q1")),
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "read", Arguments: `{}`})),
		chat.NewToolMessage(chat.ToolResult{ID: "c1", Name: "read", Result: big}),
		chat.NewAssistantMessage(chat.NewTextPart("a1")),
		chat.NewUserMessage(chat.NewTextPart("q2")),
		chat.NewAssistantMessage(chat.NewTextPart("done")),
	)
	model := newTextStubModel("SUMMARY")
	client, _ := chatclient.New(model)
	// Count trigger at the message count → a body trim can't clear it.
	c := NewCompactor(store, constClient(client), CompactionConfig{MaxMessages: 6, KeepRecent: 2})

	res, err := c.MaybeCompact(t.Context(), sessID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("a count-triggered compaction the trim can't clear must run the LLM summary")
	}
	if model.calls == 0 {
		t.Fatal("expected the LLM summary rung to fire")
	}
	after, _ := store.Read(t.Context(), sessID)
	if after[0].Role != chat.RoleSystem {
		t.Fatalf("after[0] role = %q, want the system summary", after[0].Role)
	}
}

// TestTrimForBudget_PreviewsOldNotRecentAndDoesNotMutate exercises the trim
// primitive directly: oversized args become valid JSON, oversized old results
// become previews, the recent window is untouched, and the input's shared parts
// are never mutated (copy-on-write).
func TestTrimForBudget_PreviewsOldNotRecentAndDoesNotMutate(t *testing.T) {
	bigArgs := strings.Repeat("a", 5_000)
	bigResult := strings.Repeat("b", 5_000)
	msgs := []chat.Message{
		chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "c1", Name: "write", Arguments: bigArgs})), // [0] old
		chat.NewToolMessage(chat.ToolResult{ID: "c1", Name: "write", Result: bigResult}),                           // [1] old
		chat.NewToolMessage(chat.ToolResult{ID: "c2", Name: "read", Result: bigResult}),                            // [2] recent
		chat.NewAssistantMessage(chat.NewTextPart("x")),                                                            // [3] recent
	}
	c := NewCompactor(nil, nil, CompactionConfig{KeepRecent: 2}) // boundary = 4-2 = 2

	trimmed, changed := c.trimForBudget(msgs)
	if !changed {
		t.Fatal("expected the old oversized parts to be trimmed")
	}

	gotArgs := trimmed[0].Parts[0].ToolCall.Arguments
	if len(gotArgs) >= len(bigArgs) || !strings.Contains(gotArgs, "_trimmed") {
		t.Fatalf("args not trimmed: %q", gotArgs)
	}
	if !json.Valid([]byte(gotArgs)) {
		t.Fatalf("trimmed args must stay valid JSON, got %q", gotArgs)
	}
	if got := trimmed[1].Parts[0].ToolResult.Result; len(got) >= len(bigResult) || !strings.Contains(got, "trimmed on compaction") {
		t.Fatalf("old result not previewed: len %d", len(got))
	}
	if got := trimmed[2].Parts[0].ToolResult.Result; got != bigResult {
		t.Fatal("recent result must be left full")
	}
	// The source slice's parts must be untouched (copy-on-write).
	if msgs[0].Parts[0].ToolCall.Arguments != bigArgs || msgs[1].Parts[0].ToolResult.Result != bigResult {
		t.Fatal("trimForBudget mutated its input's shared parts")
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
	calls int // Call invocations, so a test can assert the LLM rung did / didn't fire
}

func newTextStubModel(reply string) *textStubModel {
	return &textStubModel{reply: reply}
}

func (m *textStubModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	m.calls++
	message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func (m *textStubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}
