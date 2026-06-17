package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

// TestProcessState_FirstTerminalWins pins the "first terminal wins" gate that
// keeps a run loop's natural terminal (completeForGoal / failProcess / ...)
// from clobbering an external KillProcess (and vice versa), and stops a killed
// process from being resurrected into Waiting/Paused. setStatus reports whether
// THIS call performed the transition so the caller publishes a terminal event
// only when it won — no duplicate / conflicting terminals.
func TestProcessState_FirstTerminalWins(t *testing.T) {
	s := newProcessState()
	if !s.makeRunning() {
		t.Fatal("makeRunning from NotStarted should win the loop")
	}

	// First terminal write wins and reports it.
	if !s.setStatus(core.StatusKilled) {
		t.Fatal("setStatus to the first terminal should report won=true")
	}
	if got := s.getStatus(); got != core.StatusKilled {
		t.Fatalf("status = %v, want Killed", got)
	}

	// A later terminal write is refused — neither clobbers the status nor
	// reports a (would-be-duplicate-publishing) win.
	if s.setStatus(core.StatusCompleted) {
		t.Fatal("setStatus over an existing terminal should report won=false")
	}
	if got := s.getStatus(); got != core.StatusKilled {
		t.Fatalf("status = %v, want Killed (first terminal wins, not clobbered)", got)
	}

	// A non-terminal write over a terminal is also refused — a killed process
	// must not be resurrected into Waiting (which makeRunning would then resume).
	if s.setStatus(core.StatusWaiting) {
		t.Fatal("setStatus(Waiting) over a terminal should report won=false")
	}
	if got := s.getStatus(); got != core.StatusKilled {
		t.Fatalf("status = %v, want Killed (no resurrection)", got)
	}

	// markKilled is the same gate (external KillProcess side).
	if s.markKilled() {
		t.Fatal("markKilled over an existing terminal should report won=false")
	}
}

// TestProcessState_NonTerminalTransitions confirms the gate doesn't impede the
// normal Running ↔ Waiting cycle (HITL park / resume): a non-terminal status
// sets cleanly while not terminal, and makeRunning re-enters from Waiting.
func TestProcessState_NonTerminalTransitions(t *testing.T) {
	s := newProcessState()
	if !s.makeRunning() {
		t.Fatal("NotStarted → Running should win")
	}
	if !s.setStatus(core.StatusWaiting) {
		t.Fatal("Running → Waiting (park) should win")
	}
	if got := s.getStatus(); got != core.StatusWaiting {
		t.Fatalf("status = %v, want Waiting", got)
	}
	if !s.makeRunning() {
		t.Fatal("Waiting → Running (resume) should win")
	}
	if got := s.getStatus(); got != core.StatusRunning {
		t.Fatalf("status = %v, want Running", got)
	}
}
