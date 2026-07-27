package runtime

import (
	"context"
	"errors"
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
	if s.markKilled(nil) {
		t.Fatal("markKilled over an existing terminal should report won=false")
	}
}

func TestProcessStateLosingTerminalDoesNotChangeFailure(t *testing.T) {
	state := newProcessState()
	if !state.transition(core.StatusCompleted) {
		t.Fatal("completion did not win")
	}
	cause := errors.New("late cancellation")
	if state.markKilled(cause) {
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
	if !state.markKilled(nil) {
		t.Fatal("markKilled did not win")
	}
	if state.removable() {
		t.Fatal("terminal process remained removable while its run owned finalization")
	}
	state.endRun()
	if !state.removable() {
		t.Fatal("terminal process was not removable after run finalization")
	}
}

func TestProcessRegistryNeverReplacesAnExistingIdentity(t *testing.T) {
	registry := newProcessRegistry()
	existing := &Process{id: "process", state: newProcessState()}
	if started, err := existing.state.beginRun(); err != nil || !started {
		t.Fatalf("beginRun = (%v, %v)", started, err)
	}
	if !existing.state.markKilled(nil) {
		t.Fatal("kill did not win")
	}
	if !registry.insert(existing) {
		t.Fatal("insert existing process")
	}
	replacement := &Process{id: existing.id, state: newProcessState()}
	if registry.registerTree([]*Process{replacement}) {
		t.Fatal("registry replaced a terminal process before its run finalized")
	}
	existing.state.endRun()
	if registry.registerTree([]*Process{replacement}) {
		t.Fatal("registry replaced a terminal process after finalization")
	}
	if !registry.reserveProcesses([]*Process{existing}) || !registry.unregisterReservedTree([]*Process{existing}) {
		t.Fatal("remove existing process tree")
	}
	if !registry.registerTree([]*Process{replacement}) {
		t.Fatal("registry rejected identity after explicit removal")
	}
}

func TestProcessRegistryCannotMixRestoredAndExistingTreeGenerations(t *testing.T) {
	registry := newProcessRegistry()
	parent := &Process{id: "parent", state: newProcessState()}
	child := &Process{id: "child", parentID: parent.id, depth: 1, state: newProcessState()}
	if !parent.state.transition(core.StatusCompleted) {
		t.Fatal("complete parent")
	}
	if !registry.insert(parent) || !registry.insert(child) {
		t.Fatal("insert existing tree")
	}

	replacement := &Process{id: parent.id, state: newProcessState()}
	if registry.registerTree([]*Process{replacement}) {
		t.Fatal("registry replaced a parent while retaining its old descendant")
	}
	if current, ok := registry.get(parent.id); !ok || current != parent {
		t.Fatal("failed generation replacement displaced the existing parent")
	}
	if current, ok := registry.get(child.id); !ok || current != child {
		t.Fatal("failed generation replacement displaced the existing child")
	}
}

func TestProcessRegistryFailedTreeRegistrationIsAtomic(t *testing.T) {
	registry := newProcessRegistry()
	existing := &Process{id: "b"}
	if !registry.insert(existing) {
		t.Fatal("insert blocking ID")
	}

	if registry.registerTree([]*Process{{id: "a"}, {id: "b"}}) {
		t.Fatal("overlapping tree registration succeeded")
	}
	if !registry.insert(&Process{id: "a"}) {
		t.Fatal("failed tree registration published an earlier ID")
	}
}

func TestProcessRegistryTreeRemovalRejectsUnreservedDescendantAtomically(t *testing.T) {
	registry := newProcessRegistry()
	parent := &Process{id: "parent"}
	child := &Process{id: "child", parentID: parent.id}
	if !registry.insert(parent) || !registry.insert(child) {
		t.Fatal("insert process tree")
	}
	if !registry.reserveProcesses([]*Process{parent}) {
		t.Fatal("reserve parent")
	}
	if registry.unregisterReservedTree([]*Process{parent}) {
		t.Fatal("tree removal detached an unreserved child")
	}
	if current, ok := registry.get(parent.id); !ok || current != parent {
		t.Fatal("failed tree removal partially removed parent")
	}
	registry.releaseProcesses([]*Process{parent})
	if !registry.reserveProcesses([]*Process{parent, child}) {
		t.Fatal("reserve complete tree")
	}
	if !registry.unregisterReservedTree([]*Process{parent, child}) {
		t.Fatal("remove complete reserved tree")
	}
	if _, ok := registry.get(parent.id); ok {
		t.Fatal("parent remained registered")
	}
	if _, ok := registry.get(child.id); ok {
		t.Fatal("child remained registered")
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

func TestProcessStateWaitRunPrefersObservableCompletion(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	close(done)
	state := &processState{runPhase: runDriving, runDone: done}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := state.waitRun(ctx); err != nil {
		t.Fatalf("waitRun() = %v, want completed", err)
	}
}

func TestChildAdmissionIsAtomicWithParentKill(t *testing.T) {
	newTree := func(t *testing.T) (*Engine, *Process, *Process) {
		t.Helper()
		engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
		parentDef := deploymentFixture("atomic-parent", core.ConditionSet{"finish": core.True}, nil)
		childDef := deploymentFixture("atomic-child", core.ConditionSet{"finish": core.True}, nil)
		parent := createProcessForTest(t, engine, parentDef, core.Bindings{}, core.ProcessOptions{})
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
		if started, err := child.beginRun(); err != nil || !started {
			t.Fatalf("begin child run = (%v, %v)", started, err)
		}
		return engine, parent, child
	}

	t.Run("admission linearizes first", func(t *testing.T) {
		engine, parent, child := newTree(t)
		if err := engine.attachChild(parent, child); err != nil {
			t.Fatalf("attach child: %v", err)
		}
		if err := engine.Kill(t.Context(), parent.ID()); err != nil {
			t.Fatalf("kill parent: %v", err)
		}
		parent.state.endRun()
		child.state.endRun()
		if child.Status() != core.StatusKilled {
			t.Fatalf("admitted child status = %s, want killed", child.Status())
		}
	})

	t.Run("kill linearizes first", func(t *testing.T) {
		engine, parent, child := newTree(t)
		if err := engine.Kill(t.Context(), parent.ID()); err != nil {
			t.Fatalf("kill parent: %v", err)
		}
		attachErr := engine.attachChild(parent, child)
		parent.state.endRun()
		child.state.endRun()
		if !errors.Is(attachErr, ErrChildParentInactive) {
			t.Fatalf("attach error = %v, want ErrChildParentInactive", attachErr)
		}
		if _, exists := engine.Process(child.ID()); exists {
			t.Fatal("rejected child was published")
		}
	})
}
