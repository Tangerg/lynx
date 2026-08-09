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
		BlockDelta{BlockID: "answer", Text: "world"},
		BlockCompleted{Block: Block{ID: "answer", Kind: BlockAssistant, Text: "hello world"}},
		PlanChanged{Items: []PlanItem{{Title: "verify", Status: PlanDone}}},
		RunFinished{Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 10}},
	}
	for _, event := range events {
		if err := c.Apply(event); err != nil {
			t.Fatalf("Apply(%T): %v", event, err)
		}
	}
	blocks := c.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "hello world" {
		t.Fatalf("blocks = %+v", blocks)
	}
	if c.Busy() || c.Outcome().Status != OutcomeCompleted || c.Usage().InputTokens != 10 {
		t.Fatalf("conversation did not settle: phase=%v outcome=%+v usage=%+v", c.Phase(), c.Outcome(), c.Usage())
	}
}

func TestConversationRejectsAnOrphanDelta(t *testing.T) {
	c := NewConversation()
	before := c.Revision()
	err := c.Apply(BlockDelta{BlockID: "missing", Text: "lost"})
	if !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("err = %v, want ErrUnknownBlock", err)
	}
	if c.Revision() != before {
		t.Fatal("rejected event advanced the revision")
	}
}

func TestConversationReturnsSnapshots(t *testing.T) {
	c := NewConversation()
	if err := c.Apply(BlockCompleted{Block: Block{ID: "one", Text: "original", Tool: &ToolCall{Name: "original"}}}); err != nil {
		t.Fatal(err)
	}
	blocks := c.Blocks()
	blocks[0].Text = "mutated"
	blocks[0].Tool.Name = "mutated"
	if got := c.Blocks()[0].Text; got != "original" {
		t.Fatalf("aggregate was mutated through snapshot: %q", got)
	}
	if got := c.Blocks()[0].Tool.Name; got != "original" {
		t.Fatalf("aggregate tool was mutated through snapshot: %q", got)
	}
}

func TestConversationParkAndResumeAreExplicit(t *testing.T) {
	c := NewConversation()
	if c.Resumed() {
		t.Fatal("idle conversation resumed")
	}
	if err := c.Apply(RunStarted{RunID: "run_1"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(RunParked{Approval: Approval{InterruptID: "approval_1"}}); err != nil {
		t.Fatal(err)
	}
	if !c.Resumed() || c.Phase() != Running || c.Approval().InterruptID != "" {
		t.Fatalf("resume did not clear the park: phase=%v approval=%+v", c.Phase(), c.Approval())
	}
}

func TestConversationFailureBecomesVisible(t *testing.T) {
	c := NewConversation()
	c.Starting()
	c.Failed(errors.New("connection lost"))
	if c.Busy() || c.Outcome().Status != OutcomeFailed {
		t.Fatalf("failure did not settle conversation: phase=%v outcome=%+v", c.Phase(), c.Outcome())
	}
	blocks := c.Blocks()
	if len(blocks) != 1 || blocks[0].Kind != BlockError || blocks[0].Text != "connection lost" {
		t.Fatalf("failure block = %+v", blocks)
	}
}

func TestStartingAnotherRunClearsThePreviousRunIdentity(t *testing.T) {
	c := NewConversation()
	if err := c.Apply(RunStarted{RunID: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(RunFinished{Outcome: Outcome{Status: OutcomeCompleted}}); err != nil {
		t.Fatal(err)
	}
	c.Starting()
	if c.RunID() != "" {
		t.Fatalf("new run inherited id %q", c.RunID())
	}
	if err := c.Apply(RunStarted{RunID: "new"}); err != nil {
		t.Fatalf("new run could not start: %v", err)
	}
}

func TestConversationRejectsOverlappingRunStarts(t *testing.T) {
	c := NewConversation()
	if err := c.Apply(RunStarted{RunID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(RunStarted{RunID: "second"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}
