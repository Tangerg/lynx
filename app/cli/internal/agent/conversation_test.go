package agent

import (
	"errors"
	"strconv"
	"testing"
)

func TestConversationFoldsACompleteRun(t *testing.T) {
	c := NewConversation()
	events := []Event{
		RunStarted{RunID: "run", SessionID: "session"},
		BlockStarted{Block: Block{ID: "answer", Kind: BlockAssistant}},
		BlockDelta{BlockID: "answer", Text: "hello "},
		BlockCompleted{Block: Block{ID: "answer", Kind: BlockAssistant, Text: "hello world"}},
		PlanChanged{Items: []PlanItem{{Title: "verify", Status: PlanDone}}},
		RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 12}},
	}
	cursor := Cursor(1)
	for i, event := range events {
		result, err := c.ApplyEnvelope(testEnvelope(cursor, event))
		if err != nil {
			t.Fatalf("ApplyEnvelope(%T): %v", event, err)
		}
		if !result.Applied {
			t.Fatalf("event %d was treated as a duplicate", i+1)
		}
		cursor++
	}
	if c.Phase() != ConversationIdle || c.Cursor() != Cursor(len(events)) {
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
	original := testEnvelope(1, RunStarted{RunID: "run", SessionID: "session"})
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

func TestConversationInterruptAndResumeAreExplicit(t *testing.T) {
	c := NewConversation()
	approval := Approval{InterruptID: "approval_1", Title: "edit file"}
	cursor := Cursor(1)
	for _, event := range []Event{
		RunStarted{RunID: "run", SessionID: "session"},
		RunInterrupted{Interaction: approval},
		RunResumed{InterruptID: approval.InterruptID},
		RunFinished{Outcome: Outcome{Status: OutcomeCompleted}},
	} {
		if _, err := c.ApplyEnvelope(testEnvelope(cursor, event)); err != nil {
			t.Fatalf("ApplyEnvelope(%T): %v", event, err)
		}
		cursor++
	}
	if c.Phase() != ConversationIdle || c.Interaction() != nil {
		t.Fatalf("settled conversation = phase %v interaction %+v", c.Phase(), c.Interaction())
	}
}

func TestConversationClonesNestedState(t *testing.T) {
	c := NewConversation()
	exit := 0
	tool := &ToolCall{Kind: ToolUnknown, Name: "shell", Status: ToolOK, Output: "safe", ExitCode: &exit}
	if err := c.apply(RunStarted{RunID: "run", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockCompleted{Block: Block{ID: "tool", Kind: BlockTool, Tool: tool}}); err != nil {
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

	question := Question{InterruptID: "q", Title: "Choose", Fields: []QuestionField{{ID: "choice", Label: "Choice", Kind: QuestionSingle, Options: []QuestionOption{{Value: "a"}}}}}
	if err := c.apply(RunInterrupted{Interaction: question}); err != nil {
		t.Fatal(err)
	}
	cloned := c.Interaction().(Question)
	cloned.Fields[0].Options[0].Value = "changed"
	if got := c.Interaction().(Question).Fields[0].Options[0].Value; got != "a" {
		t.Fatalf("aggregate leaked question slices: %q", got)
	}
}

func TestConversationRejectsImpossibleTransitions(t *testing.T) {
	c := NewConversation()
	if err := c.apply(RunStarted{RunID: "run", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockDelta{BlockID: "missing", Text: "x"}); !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("unknown delta error = %v", err)
	}
	if err := c.apply(BlockCompleted{Block: Block{ID: "done", Kind: BlockAssistant, Text: "done"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockDelta{BlockID: "done", Text: "late"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("late delta error = %v", err)
	}
	idle := NewConversation()
	if err := idle.apply(RunInterrupted{Interaction: Approval{InterruptID: "a", Title: "Approve"}}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("early interrupt error = %v", err)
	}
	overlap := NewConversation()
	if err := overlap.apply(RunStarted{RunID: "first", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if err := overlap.apply(RunStarted{RunID: "second", SessionID: "session"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("overlapping run error = %v", err)
	}
}

func TestConversationStreamsToolDeltasIntoToolOutput(t *testing.T) {
	c := NewConversation()
	running := &ToolCall{Kind: ToolShell, Command: "go test ./...", Status: ToolRunning}
	for _, event := range []Event{
		RunStarted{RunID: "run", SessionID: "session"},
		BlockStarted{Block: Block{ID: "tool", Kind: BlockTool, Tool: running}},
		BlockDelta{BlockID: "tool", Text: "first\n"},
		BlockDelta{BlockID: "tool", Text: "second\n"},
	} {
		if err := c.apply(event); err != nil {
			t.Fatalf("apply %T: %v", event, err)
		}
	}
	blocks := c.Blocks()
	if len(blocks) != 1 || blocks[0].Tool == nil {
		t.Fatalf("blocks = %+v", blocks)
	}
	if got := blocks[0].Tool.Output; got != "first\nsecond\n" {
		t.Fatalf("streamed tool output = %q", got)
	}
	if blocks[0].Text != "" {
		t.Fatalf("tool delta leaked into block text: %q", blocks[0].Text)
	}

	completed := &ToolCall{Kind: ToolShell, Command: "go test ./...", Status: ToolOK, Output: "authoritative\n"}
	if err := c.apply(BlockCompleted{Block: Block{ID: "tool", Kind: BlockTool, Tool: completed}}); err != nil {
		t.Fatal(err)
	}
	if got := c.Blocks()[0].Tool.Output; got != "authoritative\n" {
		t.Fatalf("completed tool output = %q", got)
	}
}

func TestConversationRejectsDeltasForNonStreamingBlockKinds(t *testing.T) {
	c := NewConversation()
	if err := c.apply(RunStarted{RunID: "run", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockStarted{Block: Block{ID: "notice", Kind: BlockNotice, Text: "starting"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockDelta{BlockID: "notice", Text: "late"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("notice delta error = %v, want ErrInvalidTransition", err)
	}
	blocks := c.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "starting" {
		t.Fatalf("rejected delta mutated blocks: %+v", blocks)
	}
}

func TestConversationRejectsCompletionDriftAndRepeatedCompletion(t *testing.T) {
	for _, test := range []struct {
		name      string
		started   Block
		completed Block
	}{
		{
			name:      "block kind",
			started:   Block{ID: "block", Kind: BlockAssistant},
			completed: Block{ID: "block", Kind: BlockReasoning},
		},
		{
			name: "tool kind",
			started: Block{ID: "block", Kind: BlockTool, Tool: &ToolCall{
				Kind: ToolShell, Command: "true", Status: ToolRunning,
			}},
			completed: Block{ID: "block", Kind: BlockTool, Tool: &ToolCall{
				Kind: ToolEdit, Path: "a.go", Status: ToolOK,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewConversation()
			if err := c.apply(RunStarted{RunID: "run", SessionID: "session"}); err != nil {
				t.Fatal(err)
			}
			if err := c.apply(BlockStarted{Block: test.started}); err != nil {
				t.Fatal(err)
			}
			if err := c.apply(BlockCompleted{Block: test.completed}); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("completion drift error = %v", err)
			}
		})
	}

	c := NewConversation()
	if err := c.apply(RunStarted{RunID: "run", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	completed := Block{ID: "once", Kind: BlockAssistant, Text: "done"}
	if err := c.apply(BlockCompleted{Block: completed}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockCompleted{Block: completed}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("repeated completion error = %v", err)
	}
}

func TestConversationSettlesOpenBlocksWhenARunDoesNotCompleteNormally(t *testing.T) {
	for _, test := range []struct {
		name        string
		finish      func(*Conversation) error
		wantStatus  ToolStatus
		wantOutcome OutcomeStatus
	}{
		{
			name: "canceled",
			finish: func(c *Conversation) error {
				return c.apply(RunFinished{Outcome: Outcome{Status: OutcomeCanceled}})
			},
			wantStatus: ToolCanceled, wantOutcome: OutcomeCanceled,
		},
		{
			name: "failed",
			finish: func(c *Conversation) error {
				c.Failed(errors.New("stream failed"))
				return nil
			},
			wantStatus: ToolError, wantOutcome: OutcomeFailed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewConversation()
			for _, event := range []Event{
				RunStarted{RunID: "run", SessionID: "session"},
				BlockStarted{Block: Block{ID: "answer", Kind: BlockAssistant}},
				BlockDelta{BlockID: "answer", Text: "partial"},
				BlockStarted{Block: Block{ID: "tool", Kind: BlockTool, Tool: &ToolCall{
					Kind: ToolShell, Command: "long command", Status: ToolRunning,
				}}},
				BlockDelta{BlockID: "tool", Text: "some output\n"},
			} {
				if err := c.apply(event); err != nil {
					t.Fatalf("apply %T: %v", event, err)
				}
			}
			if err := test.finish(c); err != nil {
				t.Fatal(err)
			}
			blocks := c.Blocks()
			if blocks[0].Text != "partial" || blocks[1].Tool.Status != test.wantStatus || blocks[1].Tool.Output != "some output\n" {
				t.Fatalf("settled blocks = %+v", blocks)
			}
			if c.Outcome().Status != test.wantOutcome || c.Busy() {
				t.Fatalf("settled state = busy:%t outcome:%+v", c.Busy(), c.Outcome())
			}
		})
	}
}

func TestConversationRejectsCompletedOutcomeWithOpenBlocks(t *testing.T) {
	c := NewConversation()
	if err := c.apply(RunStarted{RunID: "run", SessionID: "session"}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(BlockStarted{Block: Block{ID: "answer", Kind: BlockAssistant}}); err != nil {
		t.Fatal(err)
	}
	if err := c.apply(RunFinished{Outcome: Outcome{Status: OutcomeCompleted}}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("completed outcome error = %v", err)
	}
	if !c.Busy() {
		t.Fatal("rejected completed outcome settled the run")
	}
}

func TestClearPresentationPreservesReplayCursor(t *testing.T) {
	c := NewConversation()
	cursor := Cursor(1)
	for _, event := range []Event{RunStarted{RunID: "run", SessionID: "session"}, BlockCompleted{Block: Block{ID: "x", Kind: BlockUser, Text: "x"}}, RunFinished{Outcome: Outcome{Status: OutcomeCompleted}}} {
		if _, err := c.ApplyEnvelope(testEnvelope(cursor, event)); err != nil {
			t.Fatal(err)
		}
		cursor++
	}
	c.ClearPresentation()
	if len(c.Blocks()) != 0 || c.Cursor() != 3 {
		t.Fatalf("clear left blocks=%d cursor=%d", len(c.Blocks()), c.Cursor())
	}
	if _, err := c.ApplyEnvelope(testEnvelope(4, RunStarted{RunID: "next", SessionID: "session"})); err != nil {
		t.Fatalf("next event after clear: %v", err)
	}
}

func TestConversationSettlesTransientStartCancellation(t *testing.T) {
	c := NewConversation()
	if err := c.Starting(); err != nil {
		t.Fatal(err)
	}
	if !c.Busy() || c.Phase() != ConversationRunning || c.RunID() != "" {
		t.Fatalf("starting state = busy:%t phase:%v run:%q", c.Busy(), c.Phase(), c.RunID())
	}
	if err := c.Starting(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("overlapping start error = %v", err)
	}
	if err := c.CancelStarting(); err != nil {
		t.Fatal(err)
	}
	if c.Busy() || c.Outcome().Status != OutcomeCanceled {
		t.Fatalf("canceled start = busy:%t outcome:%+v", c.Busy(), c.Outcome())
	}
	if err := c.CancelStarting(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("repeated start cancellation error = %v", err)
	}
}

func TestConversationCanStartAgainAfterTransientFailure(t *testing.T) {
	c := NewConversation()
	if err := c.Starting(); err != nil {
		t.Fatal(err)
	}
	want := errors.New("stream disconnected")
	c.Failed(want)
	blocks := c.Blocks()
	if c.Busy() || c.Outcome().Status != OutcomeFailed || c.Outcome().Error != want.Error() {
		t.Fatalf("failed start = busy:%t outcome:%+v", c.Busy(), c.Outcome())
	}
	if len(blocks) != 1 || blocks[0].Kind != BlockError || blocks[0].Text != want.Error() {
		t.Fatalf("failure blocks = %+v", blocks)
	}
	if err := c.Starting(); err != nil {
		t.Fatalf("start after failure: %v", err)
	}
	if c.Outcome().Status != "" {
		t.Fatalf("new start retained outcome %+v", c.Outcome())
	}
}

func testEnvelope(cursor Cursor, event Event) Envelope {
	runID := "run"
	if started, ok := event.(RunStarted); ok {
		runID = started.RunID
	}
	return Envelope{ID: "event_" + strconv.FormatUint(uint64(cursor), 10), Cursor: cursor, RunID: runID, SessionID: "session", Event: event}
}
