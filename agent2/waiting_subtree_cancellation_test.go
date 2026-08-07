package agent2

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestWaitingSubtreeCancellationPlanAppliesExactTreeState(t *testing.T) {
	engine, root, target, descendant := startWaitingSubtree(t)

	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "caller discarded the waiting branch",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Valid() {
		t.Fatal("cancellation plan is invalid")
	}
	if target.Status() != StatusWaiting || descendant.Status() != StatusWaiting || root.Status() != StatusWaiting {
		t.Fatal("planning changed the live tree")
	}
	canceled := plan.CanceledProcessIDs()
	if len(canceled) != 2 || canceled[0] != target.ID() || canceled[1] != descendant.ID() {
		t.Fatalf("canceled Process IDs = %v", canceled)
	}
	paused := plan.PausedProcessIDs()
	if len(paused) != 1 || paused[0] != root.ID() {
		t.Fatalf("paused Process IDs = %v", paused)
	}
	canceled[0] = ProcessID{}
	paused[0] = ProcessID{}
	if plan.CanceledProcessIDs()[0] != target.ID() || plan.PausedProcessIDs()[0] != root.ID() {
		t.Fatal("plan exposed mutable Process ID storage")
	}

	assertPlannedWaitingSubtree(t, plan, root.ID(), target.ID(), descendant.ID())
	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	targetResult := mustAwait(t, target)
	if targetResult.Status() != StatusCanceled ||
		targetResult.Termination().Cause() != TerminationCauseHostCancellation {
		t.Fatalf("target termination = %#v", targetResult.Termination())
	}
	descendantResult := mustAwait(t, descendant)
	if descendantResult.Status() != StatusCanceled ||
		descendantResult.Termination().Cause() != TerminationCauseParentCancellation {
		t.Fatalf("descendant termination = %#v", descendantResult.Termination())
	}
	if root.Status() != StatusPaused {
		t.Fatalf("root status = %s, want paused", root.Status())
	}
	live, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(live.JSON(), plan.ResultingSnapshot().JSON()) {
		t.Fatal("live tree does not equal the planned resulting snapshot")
	}
	assertRootHasCanceledChildSignal(t, live, root.ID(), target.ID())

	if err := root.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if result := mustAwait(t, root); result.Status() != StatusCompleted {
		t.Fatalf("resumed root status = %s", result.Status())
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingSubtreeCancellationRejectsStalePlanWithoutModification(t *testing.T) {
	engine, root, target, descendant := startWaitingSubtree(t)
	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "discard stale branch",
	)
	if err != nil {
		t.Fatal(err)
	}
	waitID, waiting := descendant.WaitID()
	if !waiting {
		t.Fatal("descendant has no external WaitID")
	}
	signalID, _ := ParseSignalID("signal:complete-stale-wait")
	response, _ := NewSignalRequest(signalID, waitID, []byte(`{"answer":true}`))
	if accepted, err := descendant.DeliverSignal(context.Background(), response); err != nil || !accepted {
		t.Fatalf("DeliverSignal accepted = %t, error = %v", accepted, err)
	}
	_ = mustAwait(t, root)
	before, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); !errors.Is(err, ErrWaitingSubtreeCancellationPlanStale) {
		t.Fatalf("Apply error = %v, want stale plan", err)
	}
	after, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.JSON(), after.JSON()) {
		t.Fatal("stale apply modified the live tree")
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingSubtreeCancellationPreservesUnsatisfiedChildWait(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "wait:subtree_all"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForProcessStatus(t, root, StatusWaiting)

	children := processesByChildKey(t, engine, root.ID())
	target := children["target"]
	sibling := children["sibling"]
	if target == nil || sibling == nil {
		t.Fatal("direct child keys do not contain target and sibling")
	}
	waitForProcessStatus(t, target, StatusWaiting)
	waitForProcessStatus(t, sibling, StatusPaused)
	descendants := directChildIDs(t, engine, target.ID())
	if len(descendants) != 1 {
		t.Fatalf("target descendant count = %d, want 1", len(descendants))
	}
	descendantID, _ := ParseProcessID(descendants[0])
	descendant, _ := engine.Process(descendantID)
	waitForProcessStatus(t, descendant, StatusWaiting)

	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "discard one waiting branch",
	)
	if err != nil {
		t.Fatal(err)
	}
	if paused := plan.PausedProcessIDs(); len(paused) != 0 {
		t.Fatalf("paused Process IDs = %v, want none before wait-all is satisfied", paused)
	}
	plannedRoot := snapshotByID(plan.ResultingSnapshot().ProcessSnapshots(), root.ID())
	if plannedRoot.Status() != StatusWaiting {
		t.Fatalf("planned root status = %s, want waiting", plannedRoot.Status())
	}
	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if root.Status() != StatusWaiting {
		t.Fatalf("live root status = %s, want waiting", root.Status())
	}
	if targetResult := mustAwait(t, target); targetResult.Status() != StatusCanceled {
		t.Fatalf("target status = %s", targetResult.Status())
	}

	if err := sibling.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	rootResult := mustAwait(t, root)
	rootOutput := childTestResult(t, rootResult)
	if len(rootOutput.CompletedKeys) != 2 || rootOutput.CompletedKeys[0] != "target" ||
		rootOutput.CompletedKeys[1] != "sibling" {
		t.Fatalf("completed keys = %v, want [target sibling]", rootOutput.CompletedKeys)
	}
	if siblingResult := mustAwait(t, sibling); siblingResult.Status() != StatusCompleted {
		t.Fatalf("sibling status = %s", siblingResult.Status())
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingSubtreeCancellationCanceledApplyDoesNotModifyTree(t *testing.T) {
	engine, root, target, _ := startWaitingSubtree(t)
	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "discard canceled apply",
	)
	if err != nil {
		t.Fatal(err)
	}
	before, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.ApplyWaitingSubtreeCancellation(ctx, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Apply error = %v, want context canceled", err)
	}
	after, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.JSON(), after.JSON()) {
		t.Fatal("Apply canceled before its gate modified the live tree")
	}

	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := root.Kill(context.Background(), "test cleanup"); err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingSubtreeCancellationRejectsForeignEnginePlan(t *testing.T) {
	engine, root, target, _ := startWaitingSubtree(t)
	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "discard foreign branch",
	)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := foreign.ApplyWaitingSubtreeCancellation(context.Background(), plan); !errors.Is(err, ErrInvalidWaitingSubtreeCancellationPlan) {
		t.Fatalf("foreign apply error = %v", err)
	}
	if target.Status() != StatusWaiting {
		t.Fatal("foreign apply changed the source Engine")
	}

	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := root.Kill(context.Background(), "test cleanup"); err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := foreign.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingSubtreeCancellationResultRestoresWithoutPrivateStateMutation(t *testing.T) {
	engine, root, target, descendant := startWaitingSubtree(t)
	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "restore canceled branch",
	)
	if err != nil {
		t.Fatal(err)
	}

	deployment := newChildTestDeployment(t)
	restoredEngine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	restoredRoot, err := restoredEngine.RestoreTree(
		context.Background(), deployment, plan.ResultingSnapshot(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRoot.Status() != StatusPaused {
		t.Fatalf("restored root status = %s", restoredRoot.Status())
	}
	restoredTarget, found := restoredEngine.Process(target.ID())
	if !found || restoredTarget.Status() != StatusCanceled {
		t.Fatalf("restored target found = %t, status = %s", found, restoredTarget.Status())
	}
	restoredDescendant, found := restoredEngine.Process(descendant.ID())
	if !found || restoredDescendant.Status() != StatusCanceled {
		t.Fatalf("restored descendant found = %t, status = %s", found, restoredDescendant.Status())
	}
	if err := restoredRoot.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if result := mustAwait(t, restoredRoot); result.Status() != StatusCompleted {
		t.Fatalf("restored root result = %s", result.Status())
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}

	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := root.Kill(context.Background(), "test cleanup"); err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingSubtreeCancellationRequiresNonRootWaitingTarget(t *testing.T) {
	engine, root, target, _ := startWaitingSubtree(t)
	if _, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), root.ID(), "root is not a valid target",
	); !errors.Is(err, ErrWaitingSubtreeCancellationUnavailable) {
		t.Fatalf("root target error = %v", err)
	}
	if root.Status() != StatusWaiting || target.Status() != StatusWaiting {
		t.Fatal("rejected root target changed the live tree")
	}
	plan, err := engine.PlanWaitingSubtreeCancellation(
		context.Background(), root.ID(), target.ID(), "test cleanup",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyWaitingSubtreeCancellation(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := root.Kill(context.Background(), "test cleanup"); err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	pausedEngine, pausedRoot, pausedChild := startPausedChildTree(t)
	if _, err := pausedEngine.PlanWaitingSubtreeCancellation(
		context.Background(), pausedRoot.ID(), pausedChild.ID(), "paused is not waiting",
	); !errors.Is(err, ErrWaitingSubtreeCancellationUnavailable) {
		t.Fatalf("paused target error = %v", err)
	}
	if pausedChild.Status() != StatusPaused {
		t.Fatal("rejected paused target changed the live tree")
	}
	for _, encoded := range directChildIDs(t, pausedEngine, pausedRoot.ID()) {
		id, _ := ParseProcessID(encoded)
		child, _ := pausedEngine.Process(id)
		if err := child.Resume(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	_ = mustAwait(t, pausedRoot)
	if err := pausedEngine.Close(); err != nil {
		t.Fatal(err)
	}
}

func startWaitingSubtree(t *testing.T) (*Engine, *Process, *Process, *Process) {
	t.Helper()
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "wait:subtree"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForProcessStatus(t, root, StatusWaiting)
	targets := directChildIDs(t, engine, root.ID())
	if len(targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(targets))
	}
	targetID, _ := ParseProcessID(targets[0])
	target, _ := engine.Process(targetID)
	waitForProcessStatus(t, target, StatusWaiting)
	descendants := directChildIDs(t, engine, target.ID())
	if len(descendants) != 1 {
		t.Fatalf("descendant count = %d, want 1", len(descendants))
	}
	descendantID, _ := ParseProcessID(descendants[0])
	descendant, _ := engine.Process(descendantID)
	waitForProcessStatus(t, descendant, StatusWaiting)
	return engine, root, target, descendant
}

func startPausedChildTree(t *testing.T) (*Engine, *Process, *Process) {
	t.Helper()
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "wait:paused"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	waitForProcessStatus(t, root, StatusWaiting)
	children := directChildIDs(t, engine, root.ID())
	if len(children) == 0 {
		t.Fatal("paused child tree has no child")
	}
	childID, _ := ParseProcessID(children[0])
	child, _ := engine.Process(childID)
	waitForProcessStatus(t, child, StatusPaused)
	return engine, root, child
}

func processesByChildKey(t *testing.T, engine *Engine, parentID ProcessID) map[string]*Process {
	t.Helper()
	processes := make(map[string]*Process)
	for _, encoded := range directChildIDs(t, engine, parentID) {
		processID, err := ParseProcessID(encoded)
		if err != nil {
			t.Fatal(err)
		}
		process, found := engine.Process(processID)
		if !found {
			t.Fatalf("child Process %s is missing", processID)
		}
		key, child := process.Relation().ChildKey()
		if !child {
			t.Fatalf("Process %s has no child key", processID)
		}
		processes[key.String()] = process
	}
	return processes
}

func assertPlannedWaitingSubtree(
	t *testing.T,
	plan WaitingSubtreeCancellationPlan,
	rootID ProcessID,
	targetID ProcessID,
	descendantID ProcessID,
) {
	t.Helper()
	statuses := make(map[ProcessID]Status)
	for _, snapshot := range plan.ResultingSnapshot().ProcessSnapshots() {
		statuses[snapshot.ProcessID()] = snapshot.Status()
		if snapshot.ProcessID() == descendantID {
			wire, err := snapshot.wire()
			if err != nil {
				t.Fatal(err)
			}
			for _, wait := range wire.Mailbox.Waits {
				if !wait.Closed {
					t.Fatal("canceled descendant retained an open external wait")
				}
			}
		}
	}
	if statuses[rootID] != StatusPaused || statuses[targetID] != StatusCanceled ||
		statuses[descendantID] != StatusCanceled {
		t.Fatalf("planned statuses = %v", statuses)
	}
}

func assertRootHasCanceledChildSignal(
	t *testing.T,
	tree TreeSnapshot,
	rootID ProcessID,
	targetID ProcessID,
) {
	t.Helper()
	root := snapshotByID(tree.ProcessSnapshots(), rootID)
	wire, err := root.wire()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range wire.Mailbox.Signals[wire.Mailbox.SignalCursor:] {
		completed, err := ParseChildrenCompleted(record.Signal)
		if err != nil {
			continue
		}
		for _, outcome := range completed.Outcomes() {
			if outcome.Result().ProcessID() == targetID && outcome.Result().Status() == StatusCanceled {
				return
			}
		}
	}
	t.Fatal("root mailbox has no canceled child outcome")
}
