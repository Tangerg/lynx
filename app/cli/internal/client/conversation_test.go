package client

import (
	"errors"
	"testing"
)

func TestConversationFoldsACompleteRun(t *testing.T) {
	c := NewConversation()
	events := []Event{
		RunStarted{RunID: "run_1", SessionID: "session_1"},
		BlockStarted{Block: Block{ID: "answer", Kind: BlockAssistant}},
		BlockDelta{BlockID: "answer", Text: "hello "},
		BlockCompleted{Block: Block{ID: "answer", Kind: BlockAssistant, Text: "hello world"}},
		PlanChanged{Items: []PlanItem{{Title: "verify", Status: PlanDone}}},
		RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 12}},
	}
	for i, event := range events {
		result, err := c.ApplyEnvelope(testEnvelope(i+1, event))
		if err != nil {
			t.Fatalf("ApplyEnvelope(%T): %v", event, err)
		}
		if !result.Applied {
			t.Fatalf("event %d was treated as a duplicate", i+1)
		}
	}
	if c.Phase() != Idle || c.Cursor() != Cursor(len(events)) {
		t.Fatalf("phase/cursor = %v/%d, want idle/%d", c.Phase(), c.Cursor(), len(events))
	}
	if got := c.Blocks(); len(got) != 1 || got[0].Text != "hello world" {
		t.Fatalf("blocks = %+v", got)
	}
	if c.Usage().InputTokens != 12 || c.Outcome().Status != OutcomeCompleted {
		t.Fatalf("settled state = %+v %+v", c.Outcome(), c.Usage())
	}
}

func TestConversationDeduplicatesReplayAndRejectsConflict(t *testing.T) {
	c := NewConversation()
	original := testEnvelope(1, RunStarted{RunID: "run_1"})
	if _, err := c.ApplyEnvelope(original); err != nil {
		t.Fatal(err)
	}
	replayed, err := c.ApplyEnvelope(original)
	if err != nil || replayed.Applied {
		t.Fatalf("duplicate = %+v, %v; want ignored", replayed, err)
	}
	conflict := original
	conflict.ID = "different"
	if _, err := c.ApplyEnvelope(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict error = %v, want ErrEventConflict", err)
	}
	if _, err := c.ApplyEnvelope(testEnvelope(3, RunFinished{})); !errors.Is(err, ErrEventGap) {
		t.Fatalf("gap error = %v, want ErrEventGap", err)
	}
}

func TestConversationRestoresAtomically(t *testing.T) {
	c := NewConversation()
	if _, err := c.ApplyEnvelope(testEnvelope(1, RunStarted{RunID: "kept"})); err != nil {
		t.Fatal(err)
	}
	bad := []Envelope{
		testEnvelope(1, RunStarted{RunID: "replacement"}),
		testEnvelope(3, RunFinished{}),
	}
	if err := c.Restore(bad); !errors.Is(err, ErrEventGap) {
		t.Fatalf("Restore error = %v, want ErrEventGap", err)
	}
	if c.RunID() != "kept" || c.Cursor() != 1 {
		t.Fatalf("failed restore mutated aggregate: run=%q cursor=%d", c.RunID(), c.Cursor())
	}
}

func TestConversationInterruptAndResumeAreExplicit(t *testing.T) {
	c := NewConversation()
	approval := Approval{InterruptID: "approval_1", Title: "edit file"}
	for i, event := range []Event{
		RunStarted{RunID: "run_1"},
		RunInterrupted{Interaction: approval},
		RunResumed{InterruptID: approval.InterruptID},
		RunFinished{Outcome: Outcome{Status: OutcomeCompleted}},
	} {
		if _, err := c.ApplyEnvelope(testEnvelope(i+1, event)); err != nil {
			t.Fatalf("ApplyEnvelope(%T): %v", event, err)
		}
	}
	if c.Phase() != Idle || c.Interaction() != nil {
		t.Fatalf("settled conversation = phase %v interaction %+v", c.Phase(), c.Interaction())
	}
}

func TestConversationClonesNestedState(t *testing.T) {
	c := NewConversation()
	exit := 0
	tool := &ToolCall{Kind: ToolUnknown, Name: "shell", Status: ToolOK, Output: "safe", ExitCode: &exit}
	if err := c.Apply(BlockCompleted{Block: Block{ID: "tool", Kind: BlockTool, Tool: tool}}); err != nil {
		t.Fatal(err)
	}
	tool.Output = "mutated"
	exit = 9
	blocks := c.Blocks()
	blocks[0].Tool.Output = "also mutated"
	if got := c.Blocks()[0].Tool.Output; got != "safe" {
		t.Fatalf("aggregate leaked tool pointer: %q", got)
	}
	if got := *c.Blocks()[0].Tool.ExitCode; got != 0 {
		t.Fatalf("aggregate leaked exit code pointer: %d", got)
	}

	question := Question{InterruptID: "q", Fields: []QuestionField{{ID: "choice", Options: []QuestionOption{{Value: "a"}}}}}
	if err := c.Apply(RunStarted{RunID: "run"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(RunInterrupted{Interaction: question}); err != nil {
		t.Fatal(err)
	}
	copy := c.Interaction().(Question)
	copy.Fields[0].Options[0].Value = "changed"
	if got := c.Interaction().(Question).Fields[0].Options[0].Value; got != "a" {
		t.Fatalf("aggregate leaked question slices: %q", got)
	}
}

func TestConversationRejectsImpossibleTransitions(t *testing.T) {
	c := NewConversation()
	if err := c.Apply(BlockDelta{BlockID: "missing", Text: "x"}); !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("unknown delta error = %v", err)
	}
	if err := c.Apply(RunInterrupted{Interaction: Approval{InterruptID: "a"}}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("early interrupt error = %v", err)
	}
	if err := c.Apply(RunStarted{RunID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(RunStarted{RunID: "second"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("overlapping run error = %v", err)
	}
}

func TestClearPresentationPreservesReplayCursor(t *testing.T) {
	c := NewConversation()
	for i, event := range []Event{RunStarted{RunID: "run"}, BlockCompleted{Block: Block{ID: "x", Kind: BlockUser, Text: "x"}}, RunFinished{}} {
		if _, err := c.ApplyEnvelope(testEnvelope(i+1, event)); err != nil {
			t.Fatal(err)
		}
	}
	c.ClearPresentation()
	if len(c.Blocks()) != 0 || c.Cursor() != 3 {
		t.Fatalf("clear left blocks=%d cursor=%d", len(c.Blocks()), c.Cursor())
	}
	if _, err := c.ApplyEnvelope(testEnvelope(4, RunStarted{RunID: "next"})); err != nil {
		t.Fatalf("next event after clear: %v", err)
	}
}

func testEnvelope(cursor int, event Event) Envelope {
	return Envelope{ID: "event_" + string(rune('a'+cursor-1)), Cursor: Cursor(cursor), RunID: "run", SessionID: "session", Event: event}
}
