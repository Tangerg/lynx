package server

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// TestTranslator_OpensUserMessageOnRootRun verifies a root run streams the
// user's input as the opening userMessage Item (item.started + item.completed)
// right after run.started — the event source the live view renders from, with
// an id that matches items.list on reload.
func TestTranslator_OpensUserMessageOnRootRun(t *testing.T) {
	input := []protocol.ContentBlock{{Type: "text", Text: "hello"}}
	tr := newTranslator("ses_1", "run_1", "", input, nil, "", "")

	out := tr.open()
	if len(out) != 3 {
		t.Fatalf("open() on a root run: got %d events, want 3 (run.started + userMessage started/completed)", len(out))
	}
	if out[0].Type != protocol.StreamRunStarted {
		t.Fatalf("event[0] = %s, want run.started", out[0].Type)
	}

	started, completed := out[1], out[2]
	if started.Type != protocol.StreamItemStarted || completed.Type != protocol.StreamItemCompleted {
		t.Fatalf("userMessage events = (%s, %s), want (item.started, item.completed)", started.Type, completed.Type)
	}
	for _, se := range []protocol.StreamEvent{started, completed} {
		if se.Item == nil || se.Item.Type != protocol.ItemTypeUserMessage {
			t.Fatalf("userMessage item missing or wrong type: %+v", se.Item)
		}
		if se.Item.RunID != "run_1" {
			t.Fatalf("userMessage RunID = %q, want run_1", se.Item.RunID)
		}
		if len(se.Item.Content) != 1 || se.Item.Content[0].Text != "hello" {
			t.Fatalf("userMessage content = %+v, want the input block", se.Item.Content)
		}
	}
	if started.Item.ID != completed.Item.ID {
		t.Fatalf("item.started id %q != item.completed id %q — must be one Item", started.Item.ID, completed.Item.ID)
	}
	if started.Item.Status != protocol.ItemStatusRunning || completed.Item.Status != protocol.ItemStatusCompleted {
		t.Fatalf("statuses = (%s, %s), want (running, completed)", started.Item.Status, completed.Item.Status)
	}

	// The opening user turn is emitted once — open() consumed userInput, so a
	// second open() yields run.started alone (defensive; pumpRun calls once).
	if again := tr.open(); len(again) != 1 || again[0].Type != protocol.StreamRunStarted {
		t.Fatalf("second open() re-emitted the user message: %+v", again)
	}
	// The turn-level TurnStart is a no-op (run.started comes from open()).
	if ts := tr.translate(turn.TurnStart{Model: "deepseek-v4-flash"}); ts != nil {
		t.Fatalf("turn TurnStart should be a no-op, got %+v", ts)
	}
}

// TestTranslator_RunStartedCarriesModel verifies the run.started RunRef
// surfaces the run's model so the frontend can label the run (RunRef.model).
func TestTranslator_RunStartedCarriesModel(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "claude-opus-4-8")
	out := tr.open()
	if len(out) == 0 || out[0].Type != protocol.StreamRunStarted || out[0].Run == nil {
		t.Fatalf("first event = %+v, want run.started with a RunRef", out)
	}
	if r := out[0].Run; r.Model != "claude-opus-4-8" {
		t.Errorf("RunRef model = %q, want claude-opus-4-8", r.Model)
	}
}

// TestTranslator_DeniedToolIsDistinct verifies a denied tool ends as a
// distinct terminal (incomplete + denied_by_user error), not a green success
// — so the UI can render "denied" rather than ✓.
func TestTranslator_DeniedToolIsDistinct(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	tr.translate(turn.ToolCallStart{CallID: "c1", ToolName: "shell", Arguments: `{"command":"rm -rf /"}`})
	end := tr.translate(turn.ToolCallEnd{CallID: "c1", Output: "tool call denied by user", Denied: true})
	if len(end) != 1 || end[0].Item == nil {
		t.Fatalf("toolEnd = %+v, want one item.completed", end)
	}
	it := end[0].Item
	if it.Status != protocol.ItemStatusIncomplete {
		t.Fatalf("denied tool status = %s, want incomplete (not green completed)", it.Status)
	}
	if it.Error == nil || it.Error.Type != "denied_by_user" {
		t.Fatalf("denied tool error = %+v, want type denied_by_user", it.Error)
	}
}

// TestTranslator_CommandOutputOnCompleted is the §5.2 regression: a completed
// command tool MUST carry the authoritative output (merged stdout+stderr) on
// the item's tool.result.output — not only as a toolOutput delta preview.
// Without it, history replay / reconnect (no deltas) render "(no output)"
// even though the model saw the output (API.md §4.4.2 / §5.2). The domain-
// neutral envelope carries name + arguments + a best-effort JSON result.
func TestTranslator_CommandOutputOnCompleted(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	tr.translate(turn.ToolCallStart{CallID: "c1", ToolName: "shell", Arguments: `{"command":"echo hi"}`})
	out := tr.translate(turn.ToolCallEnd{
		CallID: "c1",
		Output: `{"stdout":"hi\n","stderr":"oops","exit_code":0,"duration":"5ms"}`,
	})

	var completed *protocol.Item
	for _, se := range out {
		if se.Type == protocol.StreamItemCompleted {
			completed = se.Item
		}
	}
	if completed == nil || completed.Tool == nil {
		t.Fatalf("no completed toolCall item: %+v", out)
	}
	if completed.Tool.Name != "shell" {
		t.Fatalf("tool name = %q, want shell (name is the sole identity, §4.4)", completed.Tool.Name)
	}
	res, ok := completed.Tool.Result.(commandResult)
	if !ok {
		t.Fatalf("result = %T, want commandResult {exitCode,output,…} (§4.4.2)", completed.Tool.Result)
	}
	if got := res.Output; got != "hi\n\noops" {
		t.Fatalf("result.output = %q, want merged stdout+stderr %q", got, "hi\n\noops")
	}

	// A command with no output still carries result.output:"" (present, not
	// omitted) so the client renders an empty terminal rather than falling back
	// to a stale preview / "(no output)".
	tr2 := newTranslator("ses_1", "run_2", "", nil, nil, "", "")
	tr2.translate(turn.ToolCallStart{CallID: "c2", ToolName: "shell", Arguments: `{"command":"true"}`})
	out2 := tr2.translate(turn.ToolCallEnd{CallID: "c2", Output: `{"stdout":"","stderr":"","exit_code":0,"duration":"1ms"}`})
	for _, se := range out2 {
		if se.Type == protocol.StreamItemCompleted {
			res, ok := se.Item.Tool.Result.(commandResult)
			if !ok || res.Output != "" {
				t.Fatalf("empty-output command: result = %#v, want commandResult with output \"\"", se.Item.Tool.Result)
			}
		}
	}
}

// TestClassifyRunError maps each provider/transient failure onto its own stable
// wire symbol (with a clean, non-leaking detail) and leaves the rest
// internal_error — so the client branches on the symbol (back off / re-key /
// retry) without ever substring-matching detail, and the raw message (URL / Go
// call path) never reaches the wire.
func TestClassifyRunError(t *testing.T) {
	cases := []struct {
		name, msg, wantType string
		wantRetryable       bool   // transient (rate-limit / 5xx / timeout) → retryable; auth / 400 → not
		leakFragment        string // must NOT appear in the wire detail
	}{
		{"rate limit", `engine: run chat: POST "https://api.deepseek.com/v1": 429 Too Many Requests {"x"}`, "rate_limited", true, "deepseek.com"},
		{"auth", `POST "https://api.deepseek.com": 401 Unauthorized`, "invalid_api_key", false, "api.deepseek.com"},
		{"provider 5xx", `POST "https://api.x": 503 Service Unavailable`, "provider_unavailable", true, "api.x"},
		{"timeout", `Post "https://api.x": context deadline exceeded`, "timeout", true, "api.x"},
		{"bad request", `POST "https://api.x": 400 Bad Request invalid_request_error`, "provider_rejected", false, "api.x"},
		{"genuine internal", `engine: deploy turn agent: blackboard nil pointer`, "internal_error", false, ""},
		{"empty", ``, "internal_error", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTranslator("", "", "", nil, nil, "", "")
			got := tr.classifyRunError(c.msg)
			if got.Type != c.wantType {
				t.Fatalf("type = %q, want %q (msg=%q)", got.Type, c.wantType, c.msg)
			}
			if got.Retryable != c.wantRetryable {
				t.Fatalf("retryable = %v, want %v (msg=%q)", got.Retryable, c.wantRetryable, c.msg)
			}
			if got.Detail == "" {
				t.Fatalf("detail must not be empty")
			}
			if c.leakFragment != "" && strings.Contains(got.Detail, c.leakFragment) {
				t.Fatalf("detail leaked %q: %q", c.leakFragment, got.Detail)
			}
		})
	}
}

// TestClassifyRunError_AgentStuck verifies a stuck-agent terminal surfaces
// under its own wire symbol (keyed on the turn errCode) rather than collapsing
// to internal_error (T3.2). The classification reads errCode, not the message,
// so a provider error that merely mentions "stuck" is unaffected.
func TestClassifyRunError_AgentStuck(t *testing.T) {
	tr := newTranslator("", "", "", nil, nil, "", "")
	tr.errCode = "AGENT_STUCK"
	got := tr.classifyRunError("agent stuck — no forward progress")
	if got.Type != "agent_stuck" {
		t.Fatalf("type = %q, want agent_stuck", got.Type)
	}
	if got.Channel != protocol.ErrorChannelRun {
		t.Fatalf("channel = %q, want run", got.Channel)
	}

	// Without the code, the same loop-detection wording stays a generic
	// internal error — the symbol is gated on the code, not the text.
	plain := newTranslator("", "", "", nil, nil, "", "")
	if got := plain.classifyRunError("the agent got stuck somewhere"); got.Type != "internal_error" {
		t.Fatalf("uncoded 'stuck' message classified as %q, want internal_error", got.Type)
	}
}

// TestTranslator_Compaction verifies a CompactBoundary folds onto a compaction
// Item pair (item.started + item.completed, one id) carrying the net dropped
// count, so the client can render a "context compacted" divider (T3.1).
func TestTranslator_Compaction(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	out := tr.translate(turn.CompactBoundary{MessagesBefore: 20, MessagesAfter: 6})
	if len(out) != 2 {
		t.Fatalf("CompactBoundary → %d events, want 2 (item.started + item.completed)", len(out))
	}
	started, completed := out[0], out[1]
	if started.Type != protocol.StreamItemStarted || completed.Type != protocol.StreamItemCompleted {
		t.Fatalf("compaction events = (%s, %s), want (item.started, item.completed)", started.Type, completed.Type)
	}
	for _, se := range out {
		if se.Item == nil || se.Item.Type != protocol.ItemTypeCompaction {
			t.Fatalf("compaction item missing or wrong type: %+v", se.Item)
		}
		if se.Item.DroppedMessages != 14 {
			t.Fatalf("droppedMessages = %d, want 14 (20 − 6)", se.Item.DroppedMessages)
		}
	}
	if started.Item.ID != completed.Item.ID {
		t.Fatalf("compaction started/completed ids differ (%q vs %q) — must be one Item", started.Item.ID, completed.Item.ID)
	}
}

// TestTranslator_UsageProgress verifies a per-round UsageReported becomes an
// ephemeral run.progress carrying cumulative usage (input/output/reasoning +
// cost), with cost omitted when no pricing is configured (T1.2).
func TestTranslator_UsageProgress(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	out := tr.translate(turn.UsageReported{
		TokenUsage: turn.TokenUsage{PromptTokens: 1200, CompletionTokens: 80, ReasoningTokens: 30},
		CostUSD:    0.0125,
	})
	if len(out) != 1 || out[0].Type != protocol.StreamRunProgress {
		t.Fatalf("UsageReported → %+v, want one run.progress", out)
	}
	u := out[0].Progress.Usage
	if u == nil || u.InputTokens != 1200 || u.OutputTokens != 80 || u.ReasoningTokens != 30 {
		t.Fatalf("usage = %+v, want 1200/80/30", u)
	}
	if u.CostUSD == nil || *u.CostUSD != 0.0125 {
		t.Fatalf("costUsd = %v, want 0.0125", u.CostUSD)
	}
	// run.progress is ephemeral — it must not be replayed/persisted.
	if out[0].IsDurable() {
		t.Fatal("run.progress usage preview must be ephemeral (IsDurable=false)")
	}

	// No pricing configured (cost 0) → costUsd omitted, never a fabricated $0.
	free := tr.translate(turn.UsageReported{TokenUsage: turn.TokenUsage{PromptTokens: 10}})
	if free[0].Progress.Usage.CostUSD != nil {
		t.Fatalf("costUsd = %v, want nil when cost is 0", free[0].Progress.Usage.CostUSD)
	}
}

// TestTranslator_SteerMessage verifies a mid-run steer surfaces as a durable
// userMessage Item, closing any open assistant text first (T2.1).
func TestTranslator_SteerMessage(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	tr.translate(turn.MessageDelta{Text: "thinking…"}) // opens an assistant text item
	out := tr.translate(turn.SteerMessage{Text: "also check the tests"})
	if len(out) < 3 {
		t.Fatalf("SteerMessage → %d events, want close-text + item.started + item.completed", len(out))
	}
	started, completed := out[len(out)-2], out[len(out)-1]
	if started.Type != protocol.StreamItemStarted || completed.Type != protocol.StreamItemCompleted {
		t.Fatalf("steer events = (%s, %s), want (item.started, item.completed)", started.Type, completed.Type)
	}
	it := completed.Item
	if it == nil || it.Type != protocol.ItemTypeUserMessage {
		t.Fatalf("steer item = %+v, want userMessage", it)
	}
	if len(it.Content) != 1 || it.Content[0].Text != "also check the tests" {
		t.Fatalf("steer content = %+v, want the steered text", it.Content)
	}
	if !completed.IsDurable() {
		t.Fatal("steer userMessage item must be durable (it's a real conversation turn)")
	}
}

// TestTranslator_TodosSnapshot verifies the model's task list projects to a
// state.snapshot{todos} mapping domain Content/Status → wire text/status with a
// positional id (Lever 2).
func TestTranslator_TodosSnapshot(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	out := tr.translate(turn.TodosUpdated{Todos: []todo.Item{
		{Content: "write tests", Status: todo.StatusInProgress},
		{Content: "ship it", Status: todo.StatusPending},
	}})
	if len(out) != 1 || out[0].Type != protocol.StreamStateSnapshot {
		t.Fatalf("TodosUpdated → %+v, want one state.snapshot", out)
	}
	todos, ok := out[0].State["todos"].([]protocol.TodoSnapshot)
	if !ok || len(todos) != 2 {
		t.Fatalf("state.todos = %+v, want 2 TodoSnapshot", out[0].State["todos"])
	}
	if todos[0] != (protocol.TodoSnapshot{ID: "0", Text: "write tests", Status: "in_progress"}) {
		t.Fatalf("todo[0] = %+v, want {0, write tests, in_progress}", todos[0])
	}
}

// TestTranslator_OutcomeDurationAndBudget verifies the terminal outcome carries
// the run's wall-clock duration on every result, and a budget-exceeded terminal
// gets a precise "spent $X / $Y" detail (T3.4).
func TestTranslator_OutcomeDurationAndBudget(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	oc := tr.outcome(turn.TurnEnd{
		Reason:     turn.TurnEndBudgetExceeded,
		Duration:   1500 * time.Millisecond,
		CostUSD:    4.2,
		MaxCostUSD: 4.0,
	})
	if oc.Type != protocol.OutcomeMaxBudget {
		t.Fatalf("type = %s, want maxBudget", oc.Type)
	}
	if oc.Result == nil || oc.Result.DurationMs != 1500 {
		t.Fatalf("durationMs = %v, want 1500", oc.Result)
	}
	if !strings.Contains(oc.Detail, "$4.20") || !strings.Contains(oc.Detail, "$4.00") {
		t.Fatalf("maxBudget detail = %q, want it to mention $4.20 and $4.00", oc.Detail)
	}

	// A clean completion still carries the duration (the client shows "took
	// 0.8s"), with no budget detail.
	done := tr.outcome(turn.TurnEnd{Reason: turn.TurnEndCompleted, Duration: 800 * time.Millisecond})
	if done.Type != protocol.OutcomeCompleted || done.Result.DurationMs != 800 || done.Detail != "" {
		t.Fatalf("completed outcome = %+v (result %+v), want completed/800ms/no-detail", done, done.Result)
	}
}

// TestTranslator_OutcomeMaxSteps verifies a step-capped terminal maps to the
// dedicated maxSteps outcome with a precise "reached the N-step limit" detail
// (Lever 3).
func TestTranslator_OutcomeMaxSteps(t *testing.T) {
	tr := newTranslator("ses_1", "run_1", "", nil, nil, "", "")
	oc := tr.outcome(turn.TurnEnd{Reason: turn.TurnEndStepsExceeded, MaxSteps: 8})
	if oc.Type != protocol.OutcomeMaxSteps {
		t.Fatalf("type = %s, want maxSteps", oc.Type)
	}
	if !strings.Contains(oc.Detail, "8-step") {
		t.Fatalf("maxSteps detail = %q, want it to mention the 8-step limit", oc.Detail)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		msg  string
		want int
	}{
		{`429 Too Many Requests; retry-after: 30`, 30},
		{`rate limited, try again in 12 seconds`, 12},
		{`Retry-After 5`, 5},
		{`503 Service Unavailable`, 0},
		{`retry-after: 99999`, 0}, // over the 1h sanity cap
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.msg); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %d, want %d", c.msg, got, c.want)
		}
	}
}
