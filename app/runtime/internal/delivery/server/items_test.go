package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestReconcileLostRun verifies items.list heals a RunRef the durable history
// left at status:running when no live pump is driving it (a run lost to a
// restart/crash between segment.started and segment.finished) — it's presented as a
// terminal error(run_lost) so the client stops rendering a perpetual spinner,
// while genuinely live and already-terminal runs are left untouched.
func TestReconcileLostRun(t *testing.T) {
	s := newTestServer(&blockingRunRuntime{})
	startLiveRun(t, s, "run_live")

	// Dangling running run (no live pump) → terminal error(run_lost).
	lost := &protocol.RunRef{ID: "run_dead", SessionID: "ses_1", Status: protocol.RunStatusRunning}
	s.reconcileLostRun(lost)
	if lost.Status != protocol.RunStatusFinished {
		t.Fatalf("dangling running status = %s, want finished", lost.Status)
	}
	if lost.Outcome == nil || lost.Outcome.Type != protocol.OutcomeError {
		t.Fatalf("dangling running outcome = %+v, want error", lost.Outcome)
	}
	if lost.Outcome.Result == nil || lost.Outcome.Result.Error == nil || lost.Outcome.Result.Error.Type != "run_lost" {
		t.Fatalf("dangling running error = %+v, want type run_lost", lost.Outcome)
	}
	if lost.FinishedAt.IsZero() {
		t.Fatal("healed run must carry a finishedAt")
	}

	// A genuinely live run (still in the run table) is left untouched — the
	// table entry is set before the first persist, so a live run is never
	// seen as lost.
	live := &protocol.RunRef{ID: "run_live", SessionID: "ses_1", Status: protocol.RunStatusRunning}
	s.reconcileLostRun(live)
	if live.Status != protocol.RunStatusRunning || live.Outcome != nil {
		t.Fatalf("live run must not be reconciled: %+v", live)
	}

	// An already-terminal run is left untouched (no re-write of its outcome).
	done := &protocol.RunRef{
		ID: "run_done", SessionID: "ses_1",
		Status:  protocol.RunStatusFinished,
		Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted},
	}
	s.reconcileLostRun(done)
	if done.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("terminal run must not be reconciled: %+v", done.Outcome)
	}
}
