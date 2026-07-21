package runtime

import (
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning/goap"
)

// TestProcessState_FirstTerminalWins pins the "first terminal wins" gate that
// keeps a run loop's natural terminal (completeForGoal / failProcess / ...)
// from clobbering an external Kill (and vice versa), and stops a killed
// process from being resurrected into Waiting/Paused. transition reports whether
// THIS call performed the transition so the caller publishes a terminal event
// only when it won — no duplicate / conflicting terminals.
func TestProcessState_FirstTerminalWins(t *testing.T) {
	s := newProcessState()
	if started, err := s.beginRun(); err != nil || !started {
		t.Fatal("beginRun from NotStarted should win the loop")
	}

	// First terminal write wins and reports it.
	if !s.transition(core.StatusKilled) {
		t.Fatal("transition to the first terminal should report won=true")
	}
	if got := s.status(); got != core.StatusKilled {
		t.Fatalf("status = %v, want Killed", got)
	}

	// A later terminal write is refused — neither clobbers the status nor
	// reports a (would-be-duplicate-publishing) win.
	if s.transition(core.StatusCompleted) {
		t.Fatal("transition over an existing terminal should report won=false")
	}
	if got := s.status(); got != core.StatusKilled {
		t.Fatalf("status = %v, want Killed (first terminal wins, not clobbered)", got)
	}

	// A non-terminal write over a terminal is also refused — a killed process
	// must not be resurrected into Waiting (which beginRun would then resume).
	if s.transition(core.StatusWaiting) {
		t.Fatal("transition(Waiting) over a terminal should report won=false")
	}
	if got := s.status(); got != core.StatusKilled {
		t.Fatalf("status = %v, want Killed (no resurrection)", got)
	}

	// markKilled is the same gate (external Kill side).
	if won, _ := s.markKilled(nil); won {
		t.Fatal("markKilled over an existing terminal should report won=false")
	}
}

func TestProcessStateLosingTerminalDoesNotChangeFailure(t *testing.T) {
	state := newProcessState()
	if !state.transition(core.StatusCompleted) {
		t.Fatal("completion did not win")
	}
	cause := errors.New("late cancellation")
	if won, _ := state.markKilled(cause); won {
		t.Fatal("late kill replaced completion")
	}
	if failure := state.failure(); failure != nil {
		t.Fatalf("late kill changed completed failure to %v", failure)
	}
}

func TestProcessStateCannotBeRemovedBeforeRunReleasesOwnership(t *testing.T) {
	state := newProcessState()
	if started, err := state.beginRun(); err != nil || !started {
		t.Fatalf("beginRun = (%v, %v)", started, err)
	}
	if won, owned := state.markKilled(nil); !won || !owned {
		t.Fatalf("markKilled = (%v, %v), want winning run-owned kill", won, owned)
	}
	if state.removable() {
		t.Fatal("terminal process remained removable while its run owned finalization")
	}
	state.endRun()
	if !state.removable() {
		t.Fatal("terminal process was not removable after run finalization")
	}
}

func TestProcessRegistryCannotReplaceTerminalRunDuringFinalization(t *testing.T) {
	registry := newProcessRegistry()
	existing := &Process{id: "process", state: newProcessState()}
	if started, err := existing.state.beginRun(); err != nil || !started {
		t.Fatalf("beginRun = (%v, %v)", started, err)
	}
	if won, _ := existing.state.markKilled(nil); !won {
		t.Fatal("kill did not win")
	}
	if !registry.insert(existing) {
		t.Fatal("insert existing process")
	}
	replacement := &Process{id: existing.id, state: newProcessState()}
	if registry.registerNew(replacement) {
		t.Fatal("registry replaced a terminal process before its run finalized")
	}
	existing.state.endRun()
	if !registry.registerNew(replacement) {
		t.Fatal("registry did not accept replacement after finalization")
	}
}

// TestProcessState_NonTerminalTransitions confirms the gate doesn't impede the
// normal Running ↔ Waiting cycle (HITL park / resume): a non-terminal status
// sets cleanly while not terminal, and beginRun re-enters from Waiting.
func TestProcessState_NonTerminalTransitions(t *testing.T) {
	s := newProcessState()
	if started, err := s.beginRun(); err != nil || !started {
		t.Fatal("NotStarted → Running should win")
	}
	if !s.transition(core.StatusWaiting) {
		t.Fatal("Running → Waiting (park) should win")
	}
	if got := s.status(); got != core.StatusWaiting {
		t.Fatalf("status = %v, want Waiting", got)
	}
	s.endRun()
	if started, err := s.beginRun(); err != nil || !started {
		t.Fatal("Waiting → Running (resume) should win")
	}
	if got := s.status(); got != core.StatusRunning {
		t.Fatalf("status = %v, want Running", got)
	}
}

func TestProcessState_RestoredRunningAcquiresFreshOwnership(t *testing.T) {
	s := newProcessState()
	if !s.transition(core.StatusRunning) {
		t.Fatal("restore Running transition should succeed")
	}
	if started, err := s.beginRun(); err != nil || !started {
		t.Fatalf("beginRun from restored Running = (%v, %v), want (true, nil)", started, err)
	}
	if started, err := s.beginRun(); started || !errors.Is(err, ErrProcessRunning) {
		t.Fatalf("overlapping beginRun = (%v, %v), want ErrProcessRunning", started, err)
	}
	s.endRun()
}

func TestChildAdmissionIsAtomicWithParentKill(t *testing.T) {
	for range 100 {
		engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
		parentDef := deploymentFixture("atomic-parent", core.ConditionSet{"finish": core.True}, nil)
		childDef := deploymentFixture("atomic-child", core.ConditionSet{"finish": core.True}, nil)
		parent, err := engine.createProcess(t.Context(), parentDef, core.Bindings{}, core.ProcessOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if started, err := parent.beginRun(); err != nil || !started {
			t.Fatalf("begin parent run = (%v, %v)", started, err)
		}
		childDeployment, err := engine.Deploy(t.Context(), childDef)
		if err != nil {
			t.Fatal(err)
		}
		child, _, err := engine.buildProcessFromDeployment(childDeployment, core.Bindings{}, core.ProcessOptions{})
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		var (
			attachErr error
			killErr   error
			wait      sync.WaitGroup
		)
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			attachErr = engine.attachChild(parent, child)
		}()
		go func() {
			defer wait.Done()
			<-start
			killErr = engine.Kill(t.Context(), parent.ID())
		}()
		close(start)
		wait.Wait()
		parent.state.endRun()

		if killErr != nil {
			t.Fatal(killErr)
		}
		if attachErr == nil {
			if child.Status() != core.StatusKilled {
				t.Fatalf("admitted child status = %s, want killed", child.Status())
			}
			continue
		}
		if !errors.Is(attachErr, ErrChildParentInactive) {
			t.Fatalf("attach error = %v, want ErrChildParentInactive", attachErr)
		}
		if _, exists := engine.Process(child.ID()); exists {
			t.Fatal("rejected child was published")
		}
	}
}
