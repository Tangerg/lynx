package agent

import (
	"errors"
	"testing"
)

func TestConversationFoldsInitialAndResumedSegments(t *testing.T) {
	conversation := NewConversation()
	started := RunEvent{EventID: "opaque:start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}}
	apply(t, conversation, started)
	apply(t, conversation, RunEvent{EventID: "opaque:item-start", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{ID: "msg_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockAssistant}}})
	apply(t, conversation, RunEvent{EventID: "opaque:delta", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "msg_1", Text: "draft"}})
	if got := conversation.Checkpoint(); got != "opaque:item-start" {
		t.Fatalf("checkpoint after delta = %q", got)
	}
	apply(t, conversation, RunEvent{EventID: "opaque:item-done", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{ID: "msg_1", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "final"}}})

	interrupts := []Interaction{
		runningApproval("item_approval", "run shell"),
		Question{ItemID: "item_question", Title: "choose", Fields: []QuestionField{{Prompt: "Which?", Kind: QuestionSingle, Options: []QuestionOption{{Label: "A"}, {Label: "B"}}}}},
	}
	approval := interrupts[0].(Approval)
	apply(t, conversation, RunEvent{EventID: "approval-start", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{
		ID: approval.ItemID, RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool,
	}}})
	question := interrupts[1].(Question)
	apply(t, conversation, RunEvent{EventID: "question-done", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{
		ID: question.ItemID, RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockQuestion, Question: &question,
	}}})
	interruptedUsage := Usage{InputTokens: 10, OutputTokens: 2}
	apply(t, conversation, RunEvent{EventID: "opaque:park", RunID: "run_1", SegmentID: "seg_1", Event: RunInterrupted{Interactions: interrupts, Usage: interruptedUsage}})
	if conversation.Phase() != ConversationWaiting || len(conversation.Interactions()) != 2 || conversation.Usage() != interruptedUsage {
		t.Fatalf("waiting projection = phase %v, interactions %d, usage %+v", conversation.Phase(), len(conversation.Interactions()), conversation.Usage())
	}

	resumed := runningRun("seg_2")
	resumed.Usage = interruptedUsage
	apply(t, conversation, RunEvent{EventID: "different-space:start", RunID: "run_1", SegmentID: "seg_2", Event: SegmentStarted{Run: resumed}})
	if conversation.Phase() != ConversationRunning || conversation.SegmentID() != "seg_2" || len(conversation.Interactions()) != 0 {
		t.Fatalf("resumed projection = phase %v, segment %q", conversation.Phase(), conversation.SegmentID())
	}
	completedTool := approval.Tool.Clone()
	completedTool.Status = ToolOK
	apply(t, conversation, RunEvent{EventID: "approval-done", RunID: "run_1", SegmentID: "seg_2", Event: BlockCompleted{Block: Block{
		ID: approval.ItemID, RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockTool, Tool: &completedTool,
	}}})
	apply(t, conversation, RunEvent{EventID: "different-space:done", RunID: "run_1", SegmentID: "seg_2", Event: RunFinished{
		Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 14, OutputTokens: 4},
	}})
	if conversation.Phase() != ConversationIdle || conversation.Outcome().Status != OutcomeCompleted {
		t.Fatalf("terminal projection = phase %v, outcome %+v", conversation.Phase(), conversation.Outcome())
	}
	if blocks := conversation.Blocks(); len(blocks) != 3 || blocks[0].Text != "final" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestConversationRejectsRegressingRunUsage(t *testing.T) {
	conversation := NewConversation()
	run := runningRun("seg_1")
	run.Usage = Usage{InputTokens: 10}
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: run}})
	approval := runningApproval("approval_1", "shell")
	apply(t, conversation, RunEvent{EventID: "approval", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{
		ID: approval.ItemID, RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool,
	}}})
	interrupted := RunInterrupted{
		Interactions: []Interaction{approval},
		Usage:        Usage{InputTokens: 9},
	}
	if _, err := conversation.ApplyRunEvent(RunEvent{EventID: "wait", RunID: "run_1", SegmentID: "seg_1", Event: interrupted}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("regressing usage error = %v", err)
	}
}

func TestConversationTreatsEventIDAsOpaqueIdentity(t *testing.T) {
	conversation := NewConversation()
	event := RunEvent{EventID: "z/not-a-number", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}}
	apply(t, conversation, event)
	accepted, err := conversation.ApplyRunEvent(event.Clone())
	if err != nil || accepted.Applied {
		t.Fatalf("identical replay = %+v, %v", accepted, err)
	}
	conflict := event
	conflict.Event = SegmentStarted{Run: Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1", Provider: "mock", Model: "other"}}
	if _, err := conversation.ApplyRunEvent(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestConversationRejectsCrossSegmentAndInvalidTransitions(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}})
	_, err := conversation.ApplyRunEvent(RunEvent{EventID: "wrong", RunID: "run_1", SegmentID: "seg_2", Event: BlockCompleted{Block: Block{ID: "x", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "x"}}})
	if !errors.Is(err, ErrInvalidTransition) && err == nil {
		t.Fatal("cross-segment event was accepted")
	}
	_, err = conversation.ApplyRunEvent(RunEvent{EventID: "finish", RunID: "run_1", SegmentID: "seg_1", Event: RunFinished{Outcome: Outcome{Status: OutcomeCompleted}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConversationStartingWindow(t *testing.T) {
	conversation := NewConversation()
	if err := conversation.Starting(); err != nil {
		t.Fatal(err)
	}
	if err := conversation.Starting(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second Starting error = %v", err)
	}
	if err := conversation.CancelStarting(); err != nil {
		t.Fatal(err)
	}
	if conversation.Outcome().Status != OutcomeCanceled {
		t.Fatalf("outcome = %+v", conversation.Outcome())
	}
}

func TestConversationSettlesRunningItemsWithOutOfBandCancellation(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}})
	apply(t, conversation, RunEvent{EventID: "tool", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{
		ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool,
		Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning},
	}}})
	if err := conversation.SettleRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusFinished, Outcome: Outcome{Status: OutcomeCanceled}}); err != nil {
		t.Fatal(err)
	}
	block := conversation.Blocks()[0]
	if block.Status != BlockStatusIncomplete || block.Tool.Status != ToolCanceled {
		t.Fatalf("settled block = %+v", block)
	}
}

func TestConversationReconcilesAttachThenReadOverlap(t *testing.T) {
	conversation := NewConversation()
	snapshot := attachedReconciliationSnapshot()
	plan := snapshot.Plan
	stream := SegmentStream{RunID: "run_1", SegmentID: "seg_1", HeadEventID: "head", Events: func(func(RunEvent, error) bool) {}}
	if err := conversation.RestoreAttachedSnapshot(snapshot, stream); err != nil {
		t.Fatal(err)
	}

	ignored := []RunEvent{
		{EventID: "overlap-start", RunID: "run_1", SegmentID: "seg_1", Event: BlockStarted{Block: Block{ID: "same", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockAssistant}}},
		{EventID: "overlap-delta", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "same", Text: "duplicate preview"}},
		{EventID: "overlap-complete", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: snapshot.Transcript[1]}},
		{EventID: "overlap-plan-old", RunID: "run_1", SegmentID: "seg_1", Event: PlanChanged{Revision: 1, Items: []PlanItem{{Title: "older", Status: PlanPending}}}},
		{EventID: "overlap-plan-current", RunID: "run_1", SegmentID: "seg_1", Event: PlanChanged{Revision: 2, Items: plan}},
	}
	for _, event := range ignored {
		accepted, err := conversation.ApplyRunEvent(event)
		if err != nil {
			t.Fatalf("apply overlap %s: %v", event.EventID, err)
		}
		if accepted.Applied {
			t.Fatalf("overlap %s was folded twice", event.EventID)
		}
	}
	conflict := RunEvent{EventID: "overlap-plan-conflict", RunID: "run_1", SegmentID: "seg_1", Event: PlanChanged{Revision: 2, Items: []PlanItem{{Title: "different", Status: PlanActive}}}}
	if _, err := conversation.ApplyRunEvent(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("same-revision plan conflict = %v", err)
	}
	startConflict := RunEvent{
		EventID: "overlap-start-conflict", RunID: "run_1", SegmentID: "seg_1",
		Event: BlockStarted{Block: Block{ID: "same", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockReasoning}},
	}
	if _, err := conversation.ApplyRunEvent(startConflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("replayed start conflict = %v", err)
	}

	accepted, err := conversation.ApplyRunEvent(RunEvent{EventID: "new-delta", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "live", Text: "preview"}})
	if err != nil || !accepted.Applied {
		t.Fatalf("new live delta = %+v, %v", accepted, err)
	}
	accepted, err = conversation.ApplyRunEvent(RunEvent{EventID: "orphan-preview", RunID: "run_1", SegmentID: "seg_1", Event: BlockDelta{BlockID: "missing", Text: "preview without its transient start"}})
	if err != nil || accepted.Applied {
		t.Fatalf("cold-tail orphan preview = %+v, %v", accepted, err)
	}
	completedTool := snapshot.Transcript[2].Tool.Clone()
	completedTool.Status = ToolOK
	apply(t, conversation, RunEvent{EventID: "live-complete", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{ID: "live", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockTool, Tool: &completedTool}}})
	apply(t, conversation, RunEvent{EventID: "missing-complete", RunID: "run_1", SegmentID: "seg_1", Event: BlockCompleted{Block: Block{ID: "missing", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "authoritative"}}})
	apply(t, conversation, RunEvent{EventID: "new-plan", RunID: "run_1", SegmentID: "seg_1", Event: PlanChanged{Revision: 3, Items: []PlanItem{{Title: "done", Status: PlanDone}}}})
	if conversation.PlanRevision() != 3 || conversation.Checkpoint() != "new-plan" {
		t.Fatalf("reconciled state = plan revision %d, checkpoint %q", conversation.PlanRevision(), conversation.Checkpoint())
	}
}

func attachedReconciliationSnapshot() SessionSnapshot {
	return SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionRunning, Workspace: "/tmp/demo"},
		Transcript: []Block{
			{ID: "same", RunID: "run_old", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "old"},
			{ID: "same", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "current"},
			{ID: "live", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning}},
		},
		Runs: []Run{
			{ID: "run_old", SessionID: "ses_1", Status: RunStatusFinished, Outcome: Outcome{Status: OutcomeCompleted}},
			{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"},
		},
		Plan: []PlanItem{{Title: "inspect", Status: PlanActive}}, PlanRevision: 2,
	}
}

func TestConversationRejectsOrphanPreviewOutsideColdTail(t *testing.T) {
	conversation := NewConversation()
	apply(t, conversation, RunEvent{EventID: "start", RunID: "run_1", SegmentID: "seg_1", Event: SegmentStarted{Run: runningRun("seg_1")}})
	if _, err := conversation.ApplyRunEvent(RunEvent{
		EventID: "orphan", RunID: "run_1", SegmentID: "seg_1",
		Event: BlockDelta{BlockID: "missing", Text: "bad"},
	}); !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("orphan preview error = %v", err)
	}
}

func runningRun(segmentID string) Run {
	return Run{ID: "run_1", SessionID: "ses_1", Provider: "mock", Model: "balanced", Status: RunStatusRunning, ActiveSegmentID: segmentID}
}

func apply(t *testing.T, conversation *Conversation, event RunEvent) {
	t.Helper()
	accepted, err := conversation.ApplyRunEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Applied {
		t.Fatal("event was not applied")
	}
}
