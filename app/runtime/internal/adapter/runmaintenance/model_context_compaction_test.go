package runmaintenance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/core/metadata"
)

func TestModelContextCompactionRewritesDurableHistoryAndPreservesPendingInput(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:mid-run"
	history := completeContextTurns()
	if writeErr := store.Write(t.Context(), sessionID, history...); writeErr != nil {
		t.Fatal(writeErr)
	}
	pending := chat.NewUserMessage(chat.NewTextPart("steer that has not been projected yet"))
	candidate := append(cloneMessages(history), pending)
	model := newTextStubModel("MID-RUN SUMMARY")
	client, clientErr := chatclient.New(model, chatclient.Config{})
	if clientErr != nil {
		t.Fatal(clientErr)
	}
	compactor := NewCompactor(
		store,
		constClient(client),
		nil,
		CompactionConfig{MaxMessages: len(history), KeepRecent: 2},
	)
	request := durableContextRequest(t, sessionID, candidate, 0, nil)

	result, err := compactor.CompactModelContext(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed() || !result.Summarized() {
		t.Fatalf("result = changed:%t summarized:%t", result.Changed(), result.Summarized())
	}
	effective := result.Messages()
	if len(effective) != 4 || effective[0].Role != chat.RoleSystem ||
		!strings.Contains(effective[0].Text(), "MID-RUN SUMMARY") ||
		effective[len(effective)-1].Text() != pending.Text() {
		t.Fatalf("effective context = %#v", effective)
	}
	stored, err := store.Read(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 3 || !reflect.DeepEqual(stored, effective[:len(effective)-1]) {
		t.Fatalf("durable/effective context = stored:%#v effective:%#v", stored, effective)
	}
	if len(model.requests) != 1 {
		t.Fatalf("summary model calls = %d, want one", len(model.requests))
	}
}

func TestModelContextCompactionChecksEveryCallButRewritesOnlyAtThreshold(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:threshold"
	history := completeContextTurns()
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	model := newTextStubModel("THRESHOLD SUMMARY")
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	compactor := NewCompactor(
		store,
		constClient(client),
		nil,
		CompactionConfig{MaxMessages: len(history) + 1, KeepRecent: 2},
	)
	preCompactCalls := 0
	preCompact := func(context.Context) bool {
		preCompactCalls++
		return true
	}

	below, err := compactor.CompactModelContext(
		t.Context(),
		durableContextRequest(t, sessionID, history, 0, preCompact),
	)
	if err != nil {
		t.Fatal(err)
	}
	if below.Changed() || preCompactCalls != 0 || store.rewrites != 0 || len(model.requests) != 0 {
		t.Fatalf(
			"below threshold = changed:%t hook:%d rewrites:%d summaries:%d, want no work",
			below.Changed(), preCompactCalls, store.rewrites, len(model.requests),
		)
	}

	current := chat.NewUserMessage(chat.NewTextPart("the exact threshold message stays verbatim"))
	history = append(history, current)
	if writeErr := store.Write(t.Context(), sessionID, current); writeErr != nil {
		t.Fatal(writeErr)
	}
	atThreshold, err := compactor.CompactModelContext(
		t.Context(),
		durableContextRequest(t, sessionID, history, 1, preCompact),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !atThreshold.Changed() || !atThreshold.Summarized() ||
		preCompactCalls != 1 || store.rewrites != 1 || len(model.requests) != 1 {
		t.Fatalf(
			"at threshold = changed:%t summarized:%t hook:%d rewrites:%d summaries:%d",
			atThreshold.Changed(), atThreshold.Summarized(), preCompactCalls,
			store.rewrites, len(model.requests),
		)
	}
	stored, err := store.Read(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored[len(stored)-1].Text() != current.Text() {
		t.Fatalf("protected threshold message = %q, want %q", stored[len(stored)-1].Text(), current.Text())
	}

	after, err := compactor.CompactModelContext(
		t.Context(),
		durableContextRequest(t, sessionID, stored, 0, preCompact),
	)
	if err != nil {
		t.Fatal(err)
	}
	if after.Changed() || preCompactCalls != 1 || store.rewrites != 1 || len(model.requests) != 1 {
		t.Fatalf(
			"after compaction = changed:%t hook:%d rewrites:%d summaries:%d, want no immediate repeat",
			after.Changed(), preCompactCalls, store.rewrites, len(model.requests),
		)
	}
}

func TestModelContextCompactionCalibratesThresholdFromProviderUsage(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:provider-calibration"
	large := strings.Repeat("x", 2_000)
	history := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("q1" + large)),
		chat.NewAssistantMessage(chat.NewTextPart("a1" + large)),
		chat.NewUserMessage(chat.NewTextPart("q2" + large)),
		chat.NewAssistantMessage(chat.NewTextPart("a2" + large)),
		chat.NewUserMessage(chat.NewTextPart("q3" + large)),
		chat.NewAssistantMessage(chat.NewTextPart("a3" + large)),
	}
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	instructions := []chat.Message{chat.NewSystemMessage("frozen instructions")}
	rawEstimate, err := estimateModelContextTokens(
		append(cloneMessages(instructions), history...),
		nil,
		chat.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	calibration, err := agentexec.NewModelContextTokenCalibration(
		int64(rawEstimate+200),
		rawEstimate,
	)
	if err != nil {
		t.Fatal(err)
	}
	model := newTextStubModel("CALIBRATED SUMMARY")
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	compactor := NewCompactor(
		store,
		constClient(client),
		nil,
		CompactionConfig{MaxMessages: 100, MaxTokens: rawEstimate + 100, KeepRecent: 2},
	)

	below, err := compactor.CompactModelContext(
		t.Context(),
		durableContextRequest(t, sessionID, history, 0, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if below.Changed() || store.rewrites != 0 || len(model.requests) != 0 {
		t.Fatalf(
			"uncalibrated context = changed:%t rewrites:%d summaries:%d, want below threshold",
			below.Changed(), store.rewrites, len(model.requests),
		)
	}

	atThreshold, err := compactor.CompactModelContext(
		t.Context(),
		durableContextRequestWithCalibration(
			t,
			sessionID,
			history,
			0,
			calibration,
			nil,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !atThreshold.Changed() || !atThreshold.Summarized() ||
		store.rewrites != 1 || len(model.requests) != 1 {
		t.Fatalf(
			"calibrated context = changed:%t summarized:%t rewrites:%d summaries:%d",
			atThreshold.Changed(), atThreshold.Summarized(), store.rewrites, len(model.requests),
		)
	}
}

func TestFirstModelContextCompactionPreservesCurrentUserMessageVerbatim(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:first-call"
	history := completeContextTurns()
	current := chat.NewUserMessage(chat.NewTextPart("current request must be seen verbatim"))
	history = append(history, current)
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	client, err := chatclient.New(newTextStubModel("OLDER TURNS"), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	compactor := NewCompactor(
		store,
		constClient(client),
		nil,
		CompactionConfig{MaxMessages: len(history), KeepRecent: 2},
	)
	request := durableContextRequest(t, sessionID, history, 1, nil)

	result, err := compactor.CompactModelContext(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	effective := result.Messages()
	if !result.Summarized() || effective[len(effective)-1].Role != chat.RoleUser ||
		effective[len(effective)-1].Text() != current.Text() {
		t.Fatalf("first-call effective context = %#v", effective)
	}
}

func TestModelContextCompactionFailsClosedWhenProtectedInputCannotFit(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:oversized-current"
	current := chat.NewUserMessage(chat.NewTextPart(strings.Repeat("x", 16_000)))
	if err := store.Write(t.Context(), sessionID, current); err != nil {
		t.Fatal(err)
	}
	model := newTextStubModel("must not summarize unseen input")
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	compactor := NewCompactor(
		store,
		constClient(client),
		nil,
		CompactionConfig{MaxMessages: 100, MaxTokens: 1_000, KeepRecent: 2},
	)
	request := durableContextRequest(t, sessionID, []chat.Message{current}, 1, nil)

	if _, compactErr := compactor.CompactModelContext(t.Context(), request); !errors.Is(compactErr, ErrModelContextCannotFit) {
		t.Fatalf("error = %v, want ErrModelContextCannotFit", compactErr)
	}
	if len(model.requests) != 0 {
		t.Fatalf("summary model calls = %d, want zero", len(model.requests))
	}
	after, err := store.Read(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Text() != current.Text() {
		t.Fatalf("protected input changed after rejection: %#v", after)
	}
}

func TestRequiredModelContextCompactionHonorsLifecycleVetoByFailingClosed(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:veto"
	history := completeContextTurns()
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	client, err := chatclient.New(newTextStubModel("must not run"), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	compactor := NewCompactor(
		store,
		constClient(client),
		nil,
		CompactionConfig{MaxMessages: len(history), KeepRecent: 2},
	)
	request := durableContextRequest(t, sessionID, history, 0, func(context.Context) bool { return false })

	if _, err := compactor.CompactModelContext(t.Context(), request); !errors.Is(err, ErrModelContextCompactionVetoed) {
		t.Fatalf("error = %v, want ErrModelContextCompactionVetoed", err)
	}
}

func TestDurableModelContextCompactionRejectsConversationDrift(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:drift"
	history := completeContextTurns()
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	candidate := cloneMessages(history)
	candidate[0] = chat.NewUserMessage(chat.NewTextPart("different"))
	compactor := NewCompactor(store, nil, nil, CompactionConfig{MaxMessages: len(history)})
	request := durableContextRequest(t, sessionID, candidate, 0, nil)

	if _, err := compactor.CompactModelContext(t.Context(), request); !errors.Is(err, ErrModelContextDiverged) {
		t.Fatalf("error = %v, want ErrModelContextDiverged", err)
	}
}

func TestDurableModelContextCompactionIgnoresProjectionMetadataDrift(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:metadata-drift"
	history := completeContextTurns()
	history[1].Metadata = metadata.Map{"approval": []byte(`"accepted"`)}
	if err := store.Write(t.Context(), sessionID, history...); err != nil {
		t.Fatal(err)
	}
	candidate := cloneMessages(history)
	candidate[1].Metadata = nil
	compactor := NewCompactor(store, nil, nil, CompactionConfig{MaxMessages: 100})
	request := durableContextRequest(t, sessionID, candidate, 0, nil)

	result, err := compactor.CompactModelContext(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed() {
		t.Fatal("metadata-only projection drift triggered compaction")
	}
}

func TestDurableModelContextCompactionAcceptsEquivalentToolMessageGrouping(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:tool-grouping"
	first := chat.ToolResult{ID: "call_1", Name: "shell", Result: "one"}
	second := chat.ToolResult{ID: "call_2", Name: "shell", Result: "two"}
	durable := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("run both")),
		chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: first.ID, Name: first.Name, Arguments: `{}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: second.ID, Name: second.Name, Arguments: `{}`}),
		),
		chat.NewToolMessage(first),
		chat.NewToolMessage(second),
	}
	if err := store.Write(t.Context(), sessionID, durable...); err != nil {
		t.Fatal(err)
	}
	pending := chat.NewUserMessage(chat.NewTextPart("continue"))
	candidate := []chat.Message{durable[0].Clone(), durable[1].Clone(), chat.NewToolMessage(first, second), pending}
	compactor := NewCompactor(store, nil, nil, CompactionConfig{MaxMessages: 100})
	request := durableContextRequest(t, sessionID, candidate, 0, nil)

	result, err := compactor.CompactModelContext(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed() || !reflect.DeepEqual(result.Messages(), candidate) {
		t.Fatalf("equivalent grouping result = changed:%t messages:%#v", result.Changed(), result.Messages())
	}
}

func TestDurableModelContextCompactionRejectsToolResultPayloadDrift(t *testing.T) {
	store := newCompactionTestStore()
	const sessionID = "session:tool-payload-drift"
	stored := chat.NewToolMessage(chat.ToolResult{ID: "call_1", Name: "shell", Result: "stored"})
	if err := store.Write(t.Context(), sessionID, stored); err != nil {
		t.Fatal(err)
	}
	candidate := []chat.Message{
		chat.NewToolMessage(chat.ToolResult{ID: "call_1", Name: "shell", Result: "candidate"}),
	}
	compactor := NewCompactor(store, nil, nil, CompactionConfig{MaxMessages: 100})
	request := durableContextRequest(t, sessionID, candidate, 0, nil)

	if _, err := compactor.CompactModelContext(t.Context(), request); !errors.Is(err, ErrModelContextDiverged) {
		t.Fatalf("error = %v, want ErrModelContextDiverged", err)
	}
}

func durableContextRequest(
	t *testing.T,
	sessionID string,
	candidate []chat.Message,
	protectedTail int,
	preCompact func(context.Context) bool,
) agentexec.ModelContextCompaction {
	return durableContextRequestWithCalibration(
		t,
		sessionID,
		candidate,
		protectedTail,
		agentexec.ModelContextTokenCalibration{},
		preCompact,
	)
}

func durableContextRequestWithCalibration(
	t *testing.T,
	sessionID string,
	candidate []chat.Message,
	protectedTail int,
	calibration agentexec.ModelContextTokenCalibration,
	preCompact func(context.Context) bool,
) agentexec.ModelContextCompaction {
	t.Helper()
	selection, err := modelref.New("anthropic", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	request, err := agentexec.NewDurableModelContextCompaction(
		sessionID,
		selection,
		[]chat.Message{chat.NewSystemMessage("frozen instructions")},
		candidate,
		nil,
		chat.Options{},
		calibration,
		protectedTail,
		preCompact,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func completeContextTurns() []chat.Message {
	return []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("q1")),
		chat.NewAssistantMessage(chat.NewTextPart("a1")),
		chat.NewUserMessage(chat.NewTextPart("q2")),
		chat.NewAssistantMessage(chat.NewTextPart("a2")),
		chat.NewUserMessage(chat.NewTextPart("q3")),
		chat.NewAssistantMessage(chat.NewTextPart("a3")),
	}
}
