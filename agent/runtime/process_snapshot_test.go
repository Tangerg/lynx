package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/storetest"
)

type ssWord struct{ Text string }
type ssWordCount struct{ Count int }

type mismatchedProcessStore struct {
	core.ProcessStore
}

func (s mismatchedProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	snapshot, err := s.ProcessStore.Load(ctx, id)
	if err == nil {
		snapshot.ID = "different-process"
	}
	return snapshot, err
}

type durablePauseAction struct{}

type blockingResumeAction struct {
	entered chan struct{}
	release chan struct{}
}

func (durablePauseAction) Metadata() core.ActionMetadata {
	input := core.NewBinding[ssWord](core.DefaultBindingName)
	output := core.NewBinding[ssWordCount](core.DefaultBindingName)
	metadata := core.ActionMetadata{
		Name:   "durable-pause",
		Inputs: []core.Binding{input}, Outputs: []core.Binding{output},
		Cost: core.FixedScore(1), Value: core.FixedScore(0),
	}
	metadata.Preconditions = core.ConditionSet{input.String(): core.True, metadata.RunCondition(): core.False}
	metadata.Effects = core.ConditionSet{output.String(): core.True, metadata.RunCondition(): core.True}
	return metadata
}

func (durablePauseAction) Execute(_ context.Context, pc *core.ProcessContext) (core.ActionStatus, error) {
	if yielded, _ := pc.Blackboard().Condition("durable-pause-yielded"); !yielded {
		pc.Blackboard().StoreCondition("durable-pause-yielded", true)
		return core.ActionPaused, nil
	}
	pc.Blackboard().Bind(ssWordCount{Count: 42})
	return core.ActionSucceeded, nil
}

func (*blockingResumeAction) Metadata() core.ActionMetadata {
	input := core.NewBinding[ssWord](core.DefaultBindingName)
	output := core.NewBinding[ssWordCount](core.DefaultBindingName)
	metadata := core.ActionMetadata{
		Name: "blocking-resume", Inputs: []core.Binding{input}, Outputs: []core.Binding{output},
		Cost: core.FixedScore(1), Value: core.FixedScore(0),
	}
	metadata.Preconditions = core.ConditionSet{input.String(): core.True, metadata.RunCondition(): core.False}
	metadata.Effects = core.ConditionSet{output.String(): core.True, metadata.RunCondition(): core.True}
	return metadata
}

func (a *blockingResumeAction) Execute(ctx context.Context, pc *core.ProcessContext) (core.ActionStatus, error) {
	if _, resumed := pc.Blackboard().Condition("blocking-resume-ready"); !resumed {
		pc.Blackboard().StoreCondition("blocking-resume-ready", true)
		return core.ActionPaused, nil
	}
	close(a.entered)
	select {
	case <-a.release:
		pc.Blackboard().Bind(ssWordCount{Count: 1})
		return core.ActionSucceeded, nil
	case <-ctx.Done():
		return core.ActionFailed, ctx.Err()
	}
}

// buildSnapshotAgent constructs a single-action agent suitable for
// snapshot/restore exercises.
func buildSnapshotAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "snapshot-agent", Actions: []agent.Action{agent.NewAction("count", func(ctx context.Context, pc *core.ProcessContext, in ssWord) (ssWordCount, error) {
		return ssWordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "word counted"})}})
}

func TestEngine_SaveProcess_NoStore(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSnapshotAgent()
	mustDeploy(t, engine, a)

	proc, err := engine.Run(
		context.Background(), a,
		core.Input(ssWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := engine.Save(context.Background(), proc.ID()); err == nil {
		t.Error("expected error when no ProcessStore configured")
	}
}

func TestEngine_SaveAndRestore_RoundTrip(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{
		BuildID:      "snapshot-round-trip-test",
		ProcessStore: store,
	})
	a := buildSnapshotAgent()
	mustDeploy(t, engine, a)

	proc, err := engine.Run(
		context.Background(), a,
		core.Input(ssWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	revision, err := engine.Save(context.Background(), proc.ID())
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if revision != 1 {
		t.Fatalf("save revision = %d, want 1", revision)
	}

	// Verify snapshot in store.
	snap, err := store.Load(context.Background(), proc.ID())
	if err != nil {
		t.Fatalf("store load: %v", err)
	}
	if snap.Deployment.Name != "snapshot-agent" || snap.Status != core.StatusCompleted {
		t.Errorf("snapshot fields wrong: %#v", snap)
	}
	if len(snap.History) == 0 {
		t.Error("expected at least 1 history entry")
	}

	// Restore on a fresh engine with the same agent deployed.
	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "snapshot-round-trip-test", ProcessStore: store})
	mustDeploy(t, engine2, buildSnapshotAgent())

	restored, err := engine2.Restore(context.Background(), proc.ID(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.ID() != proc.ID() {
		t.Errorf("id: want %q, got %q", proc.ID(), restored.ID())
	}
	if restored.Status() != core.StatusCompleted {
		t.Errorf("status: want completed, got %s", restored.Status())
	}
	if len(restored.History()) != len(snap.History) {
		t.Errorf("history len mismatch: want %d, got %d", len(snap.History), len(restored.History()))
	}
	if revision, err := engine2.Save(context.Background(), restored.ID()); err != nil || revision != 2 {
		t.Fatalf("save restored process = revision %d, err %v", revision, err)
	}

	// The restored process's blackboard should still hold the word count.
	if _, ok := core.Result[ssWordCount](restored); !ok {
		t.Error("restored blackboard lost ssWordCount")
	}
}

// TestEngine_RestoreWaitingProcess_ResumesToCompletion proves the
// full cross-restart HITL chain: a process's JSON-safe suspension is
// snapshotted, restored on a fresh engine, answered immediately, and driven
// to completion without reconstructing a closure or replaying a pre-resume
// tick.
//
// This exercises a restored process's tick loop, which the terminal-
// state round-trip tests never do.
func TestEngine_RestoreWaitingProcess_ResumesToCompletion(t *testing.T) {
	buildGate := func() *core.Agent {
		return agent.New(agent.AgentConfig{Name: "waiting-gate", Actions: []agent.Action{agent.NewAction("gate", func(ctx context.Context, _ *core.ProcessContext, _ ssWord) (ssWordCount, error) {
			approved, err := hitl.Interrupt[bool](ctx, "approval", "approve?")
			if err != nil {
				return ssWordCount{}, err
			}
			if !approved {
				return ssWordCount{Count: -1}, nil
			}
			return ssWordCount{Count: 42}, nil
		}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "gated output"})}})
	}

	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{BuildID: "snapshot-waiting-test", ProcessStore: store})
	mustDeploy(t, engine, buildGate())

	ctx := context.Background()
	proc, done, err := engine.Start(ctx, buildGate(),
		core.Input(ssWord{Text: "hi"}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-done
	if proc.Status() != core.StatusWaiting {
		t.Fatalf("after start: status = %v, want waiting", proc.Status())
	}

	// Persist the WAITING process, then walk away from the original
	// engine entirely.
	if _, err := engine.Save(ctx, proc.ID()); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Fresh engine — shares only the store.
	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "snapshot-waiting-test", ProcessStore: store})
	mustDeploy(t, engine2, buildGate())

	restored, err := engine2.Restore(ctx, proc.ID(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status() != core.StatusWaiting {
		t.Fatalf("restored status = %v, want waiting", restored.Status())
	}
	if suspension := restored.Suspension(); suspension == nil || suspension.ID != "approval" {
		t.Fatalf("restored suspension = %#v", suspension)
	}

	// Resume with approval, then drive to completion.
	if err := engine2.Resume(restored.ID(), "approval", true); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := engine2.Continue(ctx, restored.ID()); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if restored.Status() != core.StatusCompleted {
		t.Fatalf("after resume: status = %v, want completed; failure=%v", restored.Status(), restored.Failure())
	}
	out, ok := core.Result[ssWordCount](restored)
	if !ok || out.Count != 42 {
		t.Fatalf("result = %+v ok=%v, want Count=42", out, ok)
	}
}

func TestEngineRestoreResumableClassifiesBuildMismatchAndMissingSnapshot(t *testing.T) {
	buildGate := func() *core.Agent {
		return agent.New(agent.AgentConfig{Name: "resumable-gate", Actions: []agent.Action{agent.NewAction("gate", func(ctx context.Context, _ *core.ProcessContext, _ ssWord) (ssWordCount, error) {
			approved, err := hitl.Interrupt[bool](ctx, "approval", "approve?")
			if err != nil {
				return ssWordCount{}, err
			}
			if !approved {
				return ssWordCount{Count: -1}, nil
			}
			return ssWordCount{Count: 42}, nil
		}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "gated output"})}})
	}

	store := storetest.NewMemoryProcessStore()
	first := agent.MustNewEngine(runtime.Config{BuildID: "build-a", ProcessStore: store})
	mustDeploy(t, first, buildGate())
	process, done, err := first.Start(t.Context(), buildGate(),
		core.Input(ssWord{Text: "hi"}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start waiting process: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("start waiting process: %v", err)
	}
	if _, err := first.Save(t.Context(), process.ID()); err != nil {
		t.Fatalf("save waiting process: %v", err)
	}

	different := agent.MustNewEngine(runtime.Config{BuildID: "build-b", ProcessStore: store})
	mustDeploy(t, different, buildGate())
	if resumable, err := different.Resumable(t.Context(), process.ID()); err != nil || resumable {
		t.Fatalf("different build Resumable = (%v, %v), want false, nil", resumable, err)
	}
	if _, err := different.RestoreResumable(t.Context(), process.ID(), core.ProcessOptions{}); !errors.Is(err, runtime.ErrResumableSnapshotLost) ||
		!errors.Is(err, runtime.ErrDeploymentNotFound) {
		t.Fatalf("different build RestoreResumable error = %v", err)
	}

	same := agent.MustNewEngine(runtime.Config{BuildID: "build-a", ProcessStore: store})
	mustDeploy(t, same, buildGate())
	if resumable, err := same.Resumable(t.Context(), process.ID()); err != nil || !resumable {
		t.Fatalf("same build Resumable = (%v, %v), want true, nil", resumable, err)
	}
	restored, err := same.RestoreResumable(t.Context(), process.ID(), core.ProcessOptions{})
	if err != nil || restored.Status() != core.StatusWaiting {
		t.Fatalf("same build RestoreResumable = (%v, %v), want waiting process", restored, err)
	}

	if resumable, err := same.Resumable(t.Context(), "missing"); err != nil || resumable {
		t.Fatalf("missing Resumable = (%v, %v), want false, nil", resumable, err)
	}
	if _, err := same.RestoreResumable(t.Context(), "missing", core.ProcessOptions{}); !errors.Is(err, runtime.ErrResumableSnapshotLost) ||
		!errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("missing RestoreResumable error = %v", err)
	}

	mismatched := agent.MustNewEngine(runtime.Config{BuildID: "build-a", ProcessStore: mismatchedProcessStore{ProcessStore: store}})
	mustDeploy(t, mismatched, buildGate())
	if resumable, err := mismatched.Resumable(t.Context(), process.ID()); err != nil || resumable {
		t.Fatalf("mismatched store Resumable = (%v, %v), want false, nil", resumable, err)
	}
	if _, err := mismatched.RestoreResumable(t.Context(), process.ID(), core.ProcessOptions{}); !errors.Is(err, runtime.ErrResumableSnapshotLost) ||
		!errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("mismatched store RestoreResumable error = %v", err)
	}
}

// TestSnapshot_JSONRoundTrip_PreservesConcreteType pins the cross-restart
// resume fix: a typed struct binding must survive a FULL snapshot round-trip
// through JSON — exactly what a persistent store (SQLite) does — as its
// concrete Go type, not the map[string]any JSON otherwise decodes into.
// Before type-tagging the restored value was a bare map, so a resumed
// typed-action failed its input assertion and the turn errored. The
// in-memory-store tests above don't catch this: they never marshal the whole
// snapshot, only its tagged values.
func TestSnapshot_JSONRoundTrip_PreservesConcreteType(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, buildSnapshotAgent())

	proc, err := engine.Run(context.Background(), buildSnapshotAgent(),
		core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Marshal the snapshot to JSON and back — exactly what SQLite's
	// json.Marshal/Unmarshal does across a process restart.
	snapshot, err := proc.Snapshot()
	if err != nil {
		t.Fatalf("capture snapshot: %v", err)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var snap core.ProcessSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	engine2 := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine2, buildSnapshotAgent())
	restored, err := engine2.RestoreSnapshot(snap, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	// The produced output must come back as the concrete ssWordCount, not a
	// map — proving the type survived the JSON boundary.
	out, ok := core.Result[ssWordCount](restored)
	if !ok {
		t.Fatal("restored blackboard lost the concrete ssWordCount type after a JSON round-trip")
	}
	if out.Count != len("lynx") {
		t.Errorf("ssWordCount.Count = %d, want %d", out.Count, len("lynx"))
	}
}

func TestEngine_RestoreProcess_AgentNotDeployed(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{BuildID: "snapshot-missing-agent-test", ProcessStore: store})

	started := time.Now().Add(-time.Second)
	_ = store.Apply(context.Background(), core.SnapshotMutation{Writes: []core.ProcessSnapshot{{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            "orphan",
		Deployment:    core.DeploymentRef{Name: "never-deployed", Digest: "missing"},
		StartedAt:     started,
		CapturedAt:    time.Now(),
		Status:        core.StatusCompleted,
	}}})

	if _, err := engine.Restore(context.Background(), "orphan", core.ProcessOptions{}); err == nil {
		t.Error("expected error when agent not deployed")
	}
}

type undeclaredSnapshotValue struct{ Value string }
type unencodableSnapshotValue struct{ Callback func() }

func TestSnapshotStrictDurableAndExplicitTransientBlackboard(t *testing.T) {
	a := buildSnapshotAgent()
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	blackboard, err := engine.NewBlackboard()
	if err != nil {
		t.Fatalf("NewBlackboard: %v", err)
	}
	proc, err := engine.Run(t.Context(), a,
		core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{Blackboard: blackboard})
	if err != nil {
		t.Fatal(err)
	}

	blackboard.StoreTransient("runtime_handle", func() {})
	snapshot, err := proc.Snapshot()
	if err != nil {
		t.Fatalf("transient runtime handle broke snapshot: %v", err)
	}
	if _, ok := snapshot.Blackboard["runtime_handle"]; ok {
		t.Fatal("transient named value entered durable snapshot")
	}

	blackboard.Store("undeclared", undeclaredSnapshotValue{Value: "must fail"})
	if _, err := proc.Snapshot(); err == nil {
		t.Fatal("undeclared durable value was silently captured")
	}
}

func TestSnapshotRejectsDeclaredButUnencodableDurableValue(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "unencodable-snapshot", Actions: []agent.Action{agent.NewAction("produce", func(context.Context, *core.ProcessContext, ssWord) (unencodableSnapshotValue, error) {
		return unencodableSnapshotValue{Callback: func() {
		}}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[unencodableSnapshotValue](core.GoalConfig{Description: "produced"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a,
		core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proc.Snapshot(); err == nil {
		t.Fatal("declared durable value with a function was silently captured")
	}
}

func TestEngineConcurrentSaveProcessSerializesRevisions(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{BuildID: "concurrent-save", ProcessStore: store})
	a := buildSnapshotAgent()
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a,
		core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	revisions := make(chan uint64, 2)
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			revision, saveErr := engine.Save(t.Context(), proc.ID())
			revisions <- revision
			errorsOut <- saveErr
		}()
	}
	close(start)
	wait.Wait()
	close(revisions)
	close(errorsOut)

	for saveErr := range errorsOut {
		if saveErr != nil {
			t.Fatalf("concurrent Save: %v", saveErr)
		}
	}
	seen := map[uint64]bool{}
	for revision := range revisions {
		seen[revision] = true
	}
	if !seen[1] || !seen[2] || len(seen) != 2 {
		t.Fatalf("committed revisions = %v, want 1 and 2", seen)
	}
	latest, err := store.Load(t.Context(), proc.ID())
	if err != nil || latest.Revision != 2 {
		t.Fatalf("latest snapshot revision = %d, err %v", latest.Revision, err)
	}
}

func TestRestoreRejectsUnknownTaggedBlackboardType(t *testing.T) {
	a := buildSnapshotAgent()
	engine := agent.MustNewEngine(runtime.Config{BuildID: "unknown-tag"})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a,
		core.Input(ssWord{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := proc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	value := snapshot.Blackboard[core.DefaultBindingName]
	value.Type = "example.invalid.Unknown"
	snapshot.Blackboard[core.DefaultBindingName] = value

	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "unknown-tag"})
	mustDeploy(t, engine2, a)
	if _, err := engine2.RestoreSnapshot(snapshot, core.ProcessOptions{}); err == nil {
		t.Fatal("unknown durable type unexpectedly restored")
	}
}

func TestEngineRestorePausedProcessFromDurableBlackboardState(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "paused-restore", Actions: []agent.Action{durablePauseAction{}}, Goals: []*agent.Goal{agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "paused output"})}})
	const buildID = "paused-restore-build"
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: buildID})
	mustDeploy(t, engine1, a)
	proc, err := engine1.Run(t.Context(), a,
		core.Input(ssWord{Text: "input"}), core.ProcessOptions{})
	if err != nil || proc.Status() != core.StatusPaused {
		t.Fatalf("first run status=%s err=%v", proc.Status(), err)
	}
	snapshot, err := proc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var durable core.ProcessSnapshot
	if err := json.Unmarshal(body, &durable); err != nil {
		t.Fatal(err)
	}

	engine2 := agent.MustNewEngine(runtime.Config{BuildID: buildID})
	mustDeploy(t, engine2, a)
	restored, err := engine2.RestoreSnapshot(durable, core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine2.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatal(err)
	}
	output, ok := core.Result[ssWordCount](restored)
	if restored.Status() != core.StatusCompleted || !ok || output.Count != 42 {
		t.Fatalf("restored status=%s output=%#v ok=%v failure=%v", restored.Status(), output, ok, restored.Failure())
	}
}

func TestEngineRestoreRunningProcessCanContinue(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "running-restore", Actions: []agent.Action{durablePauseAction{}}, Goals: []*agent.Goal{agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "restored output"})}})
	const buildID = "running-restore-build"
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: buildID})
	mustDeploy(t, engine1, a)
	process, err := engine1.Run(t.Context(), a, core.Input(ssWord{Text: "input"}), core.ProcessOptions{})
	if err != nil || process.Status() != core.StatusPaused {
		t.Fatalf("first run status=%s err=%v", process.Status(), err)
	}
	snapshot, err := process.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	// A crash between ticks persists the durable lifecycle as Running but
	// carries no live goroutine ownership into the new Engine.
	snapshot.Status = core.StatusRunning

	engine2 := agent.MustNewEngine(runtime.Config{BuildID: buildID})
	mustDeploy(t, engine2, a)
	restored, err := engine2.RestoreSnapshot(snapshot, core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine2.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatal(err)
	}
	if restored.Status() != core.StatusCompleted {
		t.Fatalf("restored status = %s, want completed; failure=%v", restored.Status(), restored.Failure())
	}
}

func TestEngineContinueReportsOverlappingRun(t *testing.T) {
	action := &blockingResumeAction{entered: make(chan struct{}), release: make(chan struct{})}
	a := agent.New(agent.AgentConfig{Name: "continue-owner", Actions: []agent.Action{action}, Goals: []*agent.Goal{agent.NewOutputGoal[ssWordCount](core.GoalConfig{Description: "continued output"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	process, err := engine.Run(t.Context(), a, core.Input(ssWord{Text: "input"}), core.ProcessOptions{})
	if err != nil || process.Status() != core.StatusPaused {
		t.Fatalf("first run status=%s err=%v", process.Status(), err)
	}

	done := make(chan error, 1)
	go func() { done <- engine.Continue(t.Context(), process.ID()) }()
	<-action.entered
	if err := engine.Continue(t.Context(), process.ID()); !errors.Is(err, runtime.ErrProcessRunning) {
		t.Fatalf("overlapping Continue error = %v, want ErrProcessRunning", err)
	}
	asyncDone, err := engine.ContinueAsync(t.Context(), process.ID())
	if asyncDone != nil || !errors.Is(err, runtime.ErrProcessRunning) {
		t.Fatalf("overlapping ContinueAsync = %#v, %v; want nil and ErrProcessRunning", asyncDone, err)
	}
	close(action.release)
	if err := <-done; err != nil {
		t.Fatalf("owning Continue: %v", err)
	}
}

func TestEngineDiscardDeletesDurableOnlyTreeAtomically(t *testing.T) {
	started := time.Now().UTC().Add(-time.Second)
	snapshot := func(id, parentID string, depth int) core.ProcessSnapshot {
		return core.ProcessSnapshot{
			SchemaVersion: core.ProcessSnapshotSchemaVersion,
			ID:            id, ParentID: parentID, Depth: depth,
			Deployment: core.DeploymentRef{Name: "discard", Digest: "discard-digest"},
			StartedAt:  started, CapturedAt: started.Add(time.Millisecond), Status: core.StatusCompleted,
		}
	}
	store := storetest.NewMemoryProcessStore()
	writes := []core.ProcessSnapshot{
		snapshot("root", "", 0),
		snapshot("child-b", "root", 1),
		snapshot("child-a", "root", 1),
		snapshot("grandchild", "child-a", 2),
		snapshot("outside", "", 0),
	}
	if err := store.Apply(t.Context(), core.SnapshotMutation{Writes: writes}); err != nil {
		t.Fatal(err)
	}
	engine := agent.MustNewEngine(runtime.Config{ProcessStore: store})
	if err := engine.Discard(t.Context(), "root"); err != nil {
		t.Fatal(err)
	}
	ids, err := store.List(t.Context())
	if err != nil || len(ids) != 1 || ids[0] != "outside" {
		t.Fatalf("remaining snapshots = %v, err=%v", ids, err)
	}
}

func TestEngineDiscardStoreFailurePreservesWholeDurableTree(t *testing.T) {
	storeErr := errors.New("delete unavailable")
	store := newFlakyProcessStore(storeErr)
	started := time.Now().UTC().Add(-time.Second)
	writes := []core.ProcessSnapshot{
		{
			SchemaVersion: core.ProcessSnapshotSchemaVersion,
			ID:            "root", Deployment: core.DeploymentRef{Name: "discard", Digest: "discard-digest"},
			StartedAt: started, CapturedAt: started.Add(time.Millisecond), Status: core.StatusCompleted,
		},
		{
			SchemaVersion: core.ProcessSnapshotSchemaVersion,
			ID:            "child", ParentID: "root", Depth: 1,
			Deployment: core.DeploymentRef{Name: "discard", Digest: "discard-digest"},
			StartedAt:  started, CapturedAt: started.Add(time.Millisecond), Status: core.StatusCompleted,
		},
	}
	if err := store.inner.Apply(t.Context(), core.SnapshotMutation{Writes: writes}); err != nil {
		t.Fatal(err)
	}
	engine := agent.MustNewEngine(runtime.Config{ProcessStore: store})
	if err := engine.Discard(t.Context(), "root"); !errors.Is(err, storeErr) {
		t.Fatalf("Discard error = %v, want store failure", err)
	}
	ids, err := store.inner.List(t.Context())
	if err != nil || !slices.Equal(ids, []string{"child", "root"}) {
		t.Fatalf("preserved snapshots = %v, err=%v", ids, err)
	}
}
