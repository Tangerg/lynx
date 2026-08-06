package agent2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTreeSnapshotStrictlyRejectsUnknownFields(t *testing.T) {
	tree := completedTreeSnapshot(t)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(tree.JSON(), &fields); err != nil {
		t.Fatal(err)
	}
	fields["application_revision"] = json.RawMessage(`1`)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTreeSnapshot(data); err == nil {
		t.Fatal("ParseTreeSnapshot accepted an unknown application field")
	}
}

func FuzzTreeSnapshotJSONRoundTrip(f *testing.F) {
	tree := completedTreeSnapshot(f)
	f.Add([]byte(tree.JSON()))
	f.Fuzz(func(t *testing.T, data []byte) {
		parsed, err := ParseTreeSnapshot(data)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(parsed)
		if err != nil {
			t.Fatal(err)
		}
		reparsed, err := ParseTreeSnapshot(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(parsed.JSON(), reparsed.JSON()) {
			t.Fatal("TreeSnapshot changed across a strict JSON round trip")
		}
	})
}

func TestEngineCapturesAndRestoresCompleteWaitingTree(t *testing.T) {
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
	childIDs := directChildIDs(t, engine, root.ID())
	if len(childIDs) != 3 {
		t.Fatalf("child count = %d, want 3", len(childIDs))
	}
	for _, encoded := range childIDs {
		id, _ := ParseProcessID(encoded)
		child, _ := engine.Process(id)
		waitForProcessStatus(t, child, StatusPaused)
	}
	tree, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if tree.RootID() != root.ID() || len(tree.ProcessSnapshots()) != 4 {
		t.Fatalf("tree root = %s, Process count = %d", tree.RootID(), len(tree.ProcessSnapshots()))
	}
	parsed, err := ParseTreeSnapshot(tree.JSON())
	if err != nil || parsed.RootID() != tree.RootID() || len(parsed.ProcessSnapshots()) != 4 {
		t.Fatalf("parsed tree = %#v, error = %v", parsed, err)
	}

	restoredEngine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	restoredRoot, err := restoredEngine.RestoreTree(context.Background(), deployment, parsed)
	if err != nil {
		t.Fatal(err)
	}
	for _, encoded := range childIDs {
		id, _ := ParseProcessID(encoded)
		child, found := restoredEngine.Process(id)
		if !found {
			t.Fatalf("restored child %s is missing", id)
		}
		if err := child.Resume(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	restoredResult := mustAwait(t, restoredRoot)
	restoredOutput := childTestResult(t, restoredResult)
	if len(restoredOutput.CompletedKeys) != 3 {
		t.Fatalf("restored output = %#v", restoredOutput)
	}
	if len(directChildIDs(t, restoredEngine, restoredRoot.ID())) != 3 {
		t.Fatal("tree restore duplicated or lost a child")
	}
	assertNoChildWaitRegistrations(t, restoredEngine)
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}

	for _, encoded := range childIDs {
		id, _ := ParseProcessID(encoded)
		child, _ := engine.Process(id)
		if err := child.Resume(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	_ = mustAwait(t, root)
	assertNoChildWaitRegistrations(t, engine)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTreeCaptureWaitsForInflightChildEffectsToSettle(t *testing.T) {
	dispatcher := newBlockingChildDispatcher("first", "second", "third")
	t.Cleanup(dispatcher.ReleaseAll)
	deployment := newChildTestDeploymentWithDispatcher(t, dispatcher)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "wait:all"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		<-dispatcher.started
	}
	waitForProcessStatus(t, root, StatusWaiting)
	type captureResult struct {
		snapshot TreeSnapshot
		err      error
	}
	captured := make(chan captureResult, 1)
	go func() {
		snapshot, err := engine.CaptureTree(context.Background(), root.ID())
		captured <- captureResult{snapshot: snapshot, err: err}
	}()
	select {
	case result := <-captured:
		t.Fatalf("CaptureTree crossed unsettled Effects: %#v", result)
	case <-time.After(20 * time.Millisecond):
	}
	dispatcher.ReleaseAll()
	result := <-captured
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, snapshot := range result.snapshot.ProcessSnapshots() {
		wire, err := snapshot.wire()
		if err != nil {
			t.Fatal(err)
		}
		if wire.Prepared != nil {
			for _, effect := range wire.Prepared.Effects {
				if effect.Settlement == nil || effect.Settlement.Status() == SettlementStatusUnknown {
					t.Fatalf("Process %s captured an unsettled Effect", snapshot.ProcessID())
				}
			}
		}
	}
	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.RestoreTree(context.Background(), deployment, result.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restoredResult := mustAwait(t, restored); restoredResult.Status() != StatusCompleted {
		t.Fatalf("restored status = %s", restoredResult.Status())
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTreeRestoreResolvesEveryExactDeployment(t *testing.T) {
	childDeployment := newChildTestDeployment(t)
	parentDeployment := newCrossParentDeployment(t, childDeployment.Reference())
	resolver := deploymentMapResolver{childDeployment.Reference(): childDeployment}
	engine, _ := NewEngine(EngineConfig{DeploymentResolver: resolver})
	input, _ := EncodeInput(struct{}{})
	root, err := engine.Start(context.Background(), parentDeployment, input)
	if err != nil {
		t.Fatal(err)
	}
	result := mustAwait(t, root)
	output := childTestResult(t, result)
	awaitChildren(t, engine, output.ChildIDs)
	tree, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}

	withoutResolver, _ := NewEngine(EngineConfig{})
	if _, err := withoutResolver.RestoreTree(context.Background(), parentDeployment, tree); !errors.Is(err, ErrInvalidTreeSnapshot) {
		t.Fatalf("missing resolver error = %v", err)
	}
	if err := withoutResolver.Close(); err != nil {
		t.Fatal(err)
	}
	restoredEngine, _ := NewEngine(EngineConfig{DeploymentResolver: resolver})
	restored, err := restoredEngine.RestoreTree(context.Background(), parentDeployment, tree)
	if err != nil {
		t.Fatal(err)
	}
	if restoredResult := mustAwait(t, restored); restoredResult.Status() != StatusCompleted {
		t.Fatalf("restored root status = %s", restoredResult.Status())
	}
	childID, _ := ParseProcessID(output.ChildIDs[0])
	restoredChild, found := restoredEngine.Process(childID)
	if !found || restoredChild.DeploymentRef() != childDeployment.Reference() {
		t.Fatal("cross-Strategy child binding was not restored exactly")
	}
	_ = mustAwait(t, restoredChild)
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTreeRestoreReplaysPreparedChildStartWithStableIdentity(t *testing.T) {
	acknowledger := newBlockingChildStartAcknowledger()
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{PreparedStepAcknowledger: acknowledger})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "parent"})
	original, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	prepared := <-acknowledger.captured
	preparedWire, _ := prepared.wire()
	wantChildID := deriveChildProcessID(preparedWire.Prepared.Effects[0].ID)
	tree, err := newTreeSnapshot(treeSnapshotWire{
		SchemaVersion: treeSnapshotSchemaVersion,
		RootID:        prepared.ProcessID(), Processes: []Snapshot{prepared},
	})
	if err != nil {
		t.Fatal(err)
	}

	restoredEngine, _ := NewEngine(EngineConfig{})
	restored, err := restoredEngine.RestoreTree(context.Background(), deployment, tree)
	if err != nil {
		t.Fatal(err)
	}
	result := mustAwait(t, restored)
	output := childTestResult(t, result)
	if len(output.ChildIDs) != 1 || output.ChildIDs[0] != wantChildID.String() {
		t.Fatalf("restored child output = %#v, want %s", output, wantChildID)
	}
	if len(directChildIDs(t, restoredEngine, restored.ID())) != 1 {
		t.Fatal("prepared child start did not restore exactly once")
	}
	child, found := restoredEngine.Process(wantChildID)
	if !found {
		t.Fatal("stable restored child is missing")
	}
	_ = mustAwait(t, child)
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}

	acknowledger.release()
	_ = mustAwait(t, original)
	for _, encoded := range directChildIDs(t, engine, original.ID()) {
		id, _ := ParseProcessID(encoded)
		child, _ := engine.Process(id)
		_ = mustAwait(t, child)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSingleProcessRestoreRejectsTreeMembers(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(childTestInput{Mode: "recurse:1"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	tree, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshots := tree.ProcessSnapshots()
	restoredEngine, _ := NewEngine(EngineConfig{})
	for _, snapshot := range snapshots {
		if _, err := restoredEngine.Restore(context.Background(), deployment, snapshot); !errors.Is(err, ErrTreeSnapshotRequired) {
			t.Fatalf("single restore for %s error = %v", snapshot.ProcessID(), err)
		}
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTreeRestoreValidatesTerminalOutputAgainstExactDeployment(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, _ := NewEngine(EngineConfig{})
	input, _ := EncodeInput(childTestInput{Mode: "leaf"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustAwait(t, root)
	snapshot, err := root.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := snapshot.wire()
	invalidOutput, _ := EncodeOutput(struct {
		Unexpected bool `json:"unexpected"`
	}{Unexpected: true})
	wire.Output = &invalidOutput
	forged, err := newSnapshot(wire)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := newTreeSnapshot(treeSnapshotWire{
		SchemaVersion: treeSnapshotSchemaVersion,
		RootID:        forged.ProcessID(), Processes: []Snapshot{forged},
	})
	if err != nil {
		t.Fatal(err)
	}
	restoredEngine, _ := NewEngine(EngineConfig{})
	if _, err := restoredEngine.RestoreTree(context.Background(), deployment, tree); !errors.Is(err, ErrInvalidTreeSnapshot) {
		t.Fatalf("schema mismatch error = %v", err)
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

type blockingChildStartAcknowledger struct {
	captured chan Snapshot
	once     sync.Once
	gate     chan struct{}
}

func newBlockingChildStartAcknowledger() *blockingChildStartAcknowledger {
	return &blockingChildStartAcknowledger{
		captured: make(chan Snapshot, 1), gate: make(chan struct{}),
	}
}

func (acknowledger *blockingChildStartAcknowledger) AcknowledgePreparedStep(
	_ context.Context,
	snapshot Snapshot,
) error {
	wire, err := snapshot.wire()
	if err != nil || wire.Prepared == nil || len(wire.Prepared.Effects) != 1 {
		return err
	}
	operation, err := frameworkEffectOperation(wire.Prepared.Effects[0].Effect.Payload())
	if err != nil || operation != frameworkEffectStartChild {
		return err
	}
	acknowledger.once.Do(func() { acknowledger.captured <- snapshot })
	<-acknowledger.gate
	return nil
}

func (acknowledger *blockingChildStartAcknowledger) release() { close(acknowledger.gate) }

func assertNoChildWaitRegistrations(t *testing.T, engine *Engine) {
	t.Helper()
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if len(engine.childWaits) != 0 {
		t.Fatalf("active child wait registrations = %d, want 0", len(engine.childWaits))
	}
}

func completedTreeSnapshot(t testing.TB) TreeSnapshot {
	t.Helper()
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(engineTestInput{Value: "tree"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := root.Await(context.Background()); err != nil || result.Status() != StatusCompleted {
		t.Fatalf("root result = %#v, error = %v", result, err)
	}
	tree, err := engine.CaptureTree(context.Background(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	return tree
}
