package store

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestPhaseFollowsTheRun(t *testing.T) {
	s := NewSession()
	if s.Phase() != Idle || s.Busy() {
		t.Fatal("a fresh session is not idle")
	}
	s.Apply(client.RunStarted{RunID: "run_1"})
	if s.Phase() != Running || !s.Busy() {
		t.Fatalf("phase = %v, want running", s.Phase())
	}
	if got := s.RunID(); got != "run_1" {
		t.Fatalf("run = %q", got)
	}
	s.Apply(client.RunParked{Approval: client.Approval{InterruptID: "int_1", Title: "edit"}})
	if s.Phase() != Waiting || !s.Busy() {
		t.Fatalf("phase = %v, want waiting", s.Phase())
	}
	if s.Approval.Title != "edit" {
		t.Fatalf("approval = %+v", s.Approval)
	}
	s.Resumed()
	if s.Phase() != Running || s.Approval.InterruptID != "" {
		t.Fatalf("phase = %v, approval = %+v, want running with nothing pending", s.Phase(), s.Approval)
	}
	s.Apply(client.RunFinished{
		Outcome: client.Outcome{Status: client.OutcomeCompleted},
		Usage:   client.Usage{InputTokens: 10},
	})
	if s.Phase() != Idle || s.Busy() {
		t.Fatalf("phase = %v, want idle", s.Phase())
	}
	if s.Usage.InputTokens != 10 || s.Outcome.Status != client.OutcomeCompleted {
		t.Fatalf("usage = %+v, outcome = %+v", s.Usage, s.Outcome)
	}
}

func TestASessionIsBusyFromTheMomentARunIsAskedFor(t *testing.T) {
	// The gap before the runtime answers can be a whole network round trip. A session
	// that called itself idle in that gap would answer a request to stop by quitting.
	s := NewSession()
	s.Starting()
	if !s.Busy() || s.Phase() != Running {
		t.Fatalf("phase = %v, want running", s.Phase())
	}
	if s.RunID() != "" {
		t.Fatal("a run that has not reported itself has an id")
	}
	// And the stream's own report is not upset by it.
	s.Apply(client.RunStarted{RunID: "r"})
	if s.RunID() != "r" || s.Phase() != Running {
		t.Fatalf("run = %q, phase = %v", s.RunID(), s.Phase())
	}
}

func TestDeltasBuildTheBlockTheyBelongTo(t *testing.T) {
	s := NewSession()
	s.Apply(client.BlockStarted{Block: client.Block{ID: "m", Kind: client.BlockAssistant}})
	s.Apply(client.BlockDelta{BlockID: "m", Text: "one "})
	s.Apply(client.BlockDelta{BlockID: "m", Text: "two"})
	if len(s.Blocks) != 1 {
		t.Fatalf("blocks = %d, want one", len(s.Blocks))
	}
	if got := s.Blocks[0].Text; got != "one two" {
		t.Fatalf("text = %q", got)
	}
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "m", Kind: client.BlockAssistant, Text: "one two"}})
	if len(s.Blocks) != 1 {
		t.Fatalf("completing a block added another: %d blocks", len(s.Blocks))
	}
}

func TestADeltaForABlockNobodyStartedIsDropped(t *testing.T) {
	// It would mean the stream and this state have diverged. Inventing a block to
	// hang it on would hide that rather than show it.
	s := NewSession()
	s.Apply(client.BlockDelta{BlockID: "ghost", Text: "text"})
	if len(s.Blocks) != 0 {
		t.Fatalf("blocks = %+v, want none", s.Blocks)
	}
}

func TestABlockThatNeverStreamedArrivesComplete(t *testing.T) {
	s := NewSession()
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "note", Kind: client.BlockNotice, Text: "done"}})
	if len(s.Blocks) != 1 || s.Blocks[0].Text != "done" {
		t.Fatalf("blocks = %+v", s.Blocks)
	}
}

func TestBlocksKeepTheirOrder(t *testing.T) {
	s := NewSession()
	for _, id := range []string{"a", "b", "c"} {
		s.Apply(client.BlockCompleted{Block: client.Block{ID: id, Text: id}})
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := s.Blocks[i].ID; got != want {
			t.Fatalf("block %d = %q, want %q", i, got, want)
		}
	}
}

func TestRevisionCountsChanges(t *testing.T) {
	// It is how a loop decides whether to draw, so an event that changed nothing must
	// not move it.
	s := NewSession()
	before := s.Revision()
	s.Apply(client.BlockDelta{BlockID: "ghost", Text: "x"})
	if s.Revision() != before {
		t.Fatal("an event that changed nothing moved the revision")
	}
	s.Apply(client.RunStarted{RunID: "r"})
	if s.Revision() == before {
		t.Fatal("an event that changed something did not move the revision")
	}
}

func TestStartingARunClearsTheLastOutcome(t *testing.T) {
	// The previous run's verdict beside a running one would read as this run's.
	s := NewSession()
	s.Apply(client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeFailed, Error: "boom"}})
	s.Apply(client.RunStarted{RunID: "next"})
	if s.Outcome.Status != "" {
		t.Fatalf("outcome = %+v, want it cleared", s.Outcome)
	}
}

func TestFailedEndsTheRunVisibly(t *testing.T) {
	// A run that ended in a way the stream never reported still has to stop the
	// spinner and say why.
	s := NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Failed(errors.New("connection lost"))
	if s.Busy() {
		t.Fatal("still busy after the run failed")
	}
	if s.Outcome.Status != client.OutcomeFailed || s.Outcome.Error != "connection lost" {
		t.Fatalf("outcome = %+v", s.Outcome)
	}
	last := s.Blocks[len(s.Blocks)-1]
	if last.Kind != client.BlockError {
		t.Fatalf("last block = %+v, want the failure in the transcript", last)
	}
}

func TestPlanIsReplacedWholesale(t *testing.T) {
	s := NewSession()
	s.Apply(client.PlanChanged{Items: []client.PlanItem{{Title: "one"}, {Title: "two"}}})
	s.Apply(client.PlanChanged{Items: []client.PlanItem{{Title: "only"}}})
	if len(s.Plan) != 1 || s.Plan[0].Title != "only" {
		t.Fatalf("plan = %+v", s.Plan)
	}
}

func TestResetEmptiesTheSessionAndSaysSo(t *testing.T) {
	s := NewSession()
	s.Apply(client.RunStarted{RunID: "r"})
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "a", Text: "a"}})
	before := s.Revision()

	s.Reset()
	if len(s.Blocks) != 0 || s.RunID() != "" || s.Busy() {
		t.Fatalf("session after reset = %+v", s)
	}
	if s.Revision() <= before {
		t.Fatal("a reset did not count as a change, so nothing would redraw")
	}
	// And it is usable again.
	s.Apply(client.BlockCompleted{Block: client.Block{ID: "b", Text: "b"}})
	if len(s.Blocks) != 1 {
		t.Fatalf("blocks after reuse = %+v", s.Blocks)
	}
}

func TestTheZeroSessionWorks(t *testing.T) {
	var s Session
	s.Apply(client.BlockStarted{Block: client.Block{ID: "a", Kind: client.BlockAssistant}})
	s.Apply(client.BlockDelta{BlockID: "a", Text: "x"})
	if len(s.Blocks) != 1 || s.Blocks[0].Text != "x" {
		t.Fatalf("blocks = %+v", s.Blocks)
	}
}

func TestResumedOnlyMeansSomethingWhenParked(t *testing.T) {
	s := NewSession()
	before := s.Revision()
	s.Resumed()
	if s.Revision() != before {
		t.Fatal("resuming a session that was not parked changed something")
	}
}
