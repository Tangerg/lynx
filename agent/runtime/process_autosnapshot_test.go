package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/storetest"
)

type flakyProcessStore struct {
	inner *storetest.MemoryProcessStore
	fail  atomic.Bool
	err   error
}

func newFlakyProcessStore(err error) *flakyProcessStore {
	store := &flakyProcessStore{inner: storetest.NewMemoryProcessStore(), err: err}
	store.fail.Store(true)
	return store
}

func (s *flakyProcessStore) Apply(ctx context.Context, change core.ProcessSnapshotChange) error {
	if s.fail.Load() {
		return s.err
	}
	return s.inner.Apply(ctx, change)
}

func (s *flakyProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	return s.inner.Load(ctx, id)
}

func (s *flakyProcessStore) List(ctx context.Context) ([]string, error) {
	return s.inner.List(ctx)
}

func autoSnapshotAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "auto-snapshot-policy", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})
}

func autoSnapshotWaitingAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "auto-snapshot-waiting", Actions: []agent.Action{agent.NewAction("wait", func(ctx context.Context, _ *core.ProcessContext, _ word) (wordCount, error) {
		approved, err := hitl.Interrupt[bool](ctx, "approval", "approve?")
		if err != nil {
			return wordCount{}, err
		}
		if !approved {
			return wordCount{Count: -1}, nil
		}
		return wordCount{Count: 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "approved"})}})
}

// TestAutoSnapshot_PersistsTerminalState verifies that with AutoSnapshot on
// and a ProcessStore configured, a completed run is persisted automatically
// (no explicit Save call) and is loadable afterward.
func TestAutoSnapshot_PersistsTerminalState(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "snap", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{
		BuildID:      "auto-snapshot-test",
		ProcessStore: store,
		AutoSnapshot: true,
	})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(context.Background(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s", proc.Status())
	}

	snap, err := store.Load(context.Background(), proc.ID())
	if err != nil {
		t.Fatalf("auto-snapshot not persisted: %v", err)
	}
	if snap.Status != core.StatusCompleted {
		t.Fatalf("snapshot status = %s, want completed", snap.Status)
	}
	if snap.Deployment.Name != "snap" {
		t.Fatalf("snapshot deployment = %s", snap.Deployment)
	}
}

// TestAutoSnapshot_DisabledByDefault confirms the historical behavior: with
// a store configured but AutoSnapshot off, nothing is persisted unless the
// host calls Save explicitly.
func TestAutoSnapshot_DisabledByDefault(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "nosnap", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{BuildID: "auto-snapshot-test", ProcessStore: store})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(context.Background(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := store.Load(context.Background(), proc.ID()); err == nil {
		t.Fatal("expected nothing persisted when AutoSnapshot is off")
	}
}

func TestAutoSnapshotPersistsCancellationWithDetachedContext(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{
		BuildID:      "snapshot-cancellation",
		ProcessStore: store,
		AutoSnapshot: true,
	})
	a := autoSnapshotAgent()
	mustDeploy(t, engine, a)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	proc, err := engine.Run(ctx, a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if proc.Status() != core.StatusKilled || !errors.Is(proc.Failure(), context.Canceled) {
		t.Fatalf("process status=%s failure=%v", proc.Status(), proc.Failure())
	}

	snapshot, err := store.Load(t.Context(), proc.ID())
	if err != nil {
		t.Fatalf("Load canceled process: %v", err)
	}
	if snapshot.Status != core.StatusKilled {
		t.Fatalf("snapshot status = %s, want killed", snapshot.Status)
	}
}

func TestAutoSnapshotFailureDoesNotClobberTerminalCancellation(t *testing.T) {
	storeErr := errors.New("snapshot unavailable")
	store := newFlakyProcessStore(storeErr)
	engine := agent.MustNewEngine(runtime.Config{
		BuildID:      "snapshot-cancellation-failure",
		ProcessStore: store,
		AutoSnapshot: true,
	})
	a := autoSnapshotAgent()
	mustDeploy(t, engine, a)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	proc, err := engine.Run(ctx, a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, storeErr) {
		t.Fatalf("Run error = %v, want joined cancellation and snapshot failure", err)
	}
	if proc.Status() != core.StatusKilled {
		t.Fatalf("process status = %s, want killed", proc.Status())
	}
	if !errors.Is(proc.Failure(), context.Canceled) || errors.Is(proc.Failure(), storeErr) {
		t.Fatalf("process failure = %v, want cancellation only", proc.Failure())
	}
}

func TestAutoSnapshotFailurePolicyFailProcess(t *testing.T) {
	storeErr := errors.New("snapshot unavailable")
	store := newFlakyProcessStore(storeErr)
	engine := agent.MustNewEngine(runtime.Config{BuildID: "snapshot-fail", ProcessStore: store, AutoSnapshot: true})
	a := autoSnapshotAgent()
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if !errors.Is(err, storeErr) {
		t.Fatalf("Run error = %v", err)
	}
	if proc.Status() != core.StatusFailed || !errors.Is(proc.Failure(), storeErr) {
		t.Fatalf("process status=%s failure=%v", proc.Status(), proc.Failure())
	}
}

func TestAutoSnapshotFailurePolicyPauseAndRetry(t *testing.T) {
	store := newFlakyProcessStore(errors.New("snapshot unavailable"))
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "snapshot-pause", ProcessStore: store, AutoSnapshot: true,
		SnapshotFailurePolicy: runtime.SnapshotFailurePauseProcess,
	})
	a := autoSnapshotAgent()
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil || proc.Status() != core.StatusPaused {
		t.Fatalf("paused run status=%s err=%v", proc.Status(), err)
	}
	store.fail.Store(false)
	if err := engine.Continue(t.Context(), proc.ID()); err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("resumed status=%s failure=%v", proc.Status(), proc.Failure())
	}
	loaded, err := store.Load(t.Context(), proc.ID())
	if err != nil || loaded.Status != core.StatusCompleted {
		t.Fatalf("loaded = %#v, err %v", loaded, err)
	}
}

func TestAutoSnapshotFailurePolicyPreservesWaitingContinuation(t *testing.T) {
	store := newFlakyProcessStore(errors.New("snapshot unavailable"))
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "snapshot-waiting-pause", ProcessStore: store, AutoSnapshot: true,
		SnapshotFailurePolicy: runtime.SnapshotFailurePauseProcess,
	})
	a := autoSnapshotWaitingAgent()
	proc, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusWaiting || proc.Suspension() == nil {
		t.Fatalf("process status=%s suspension=%#v", proc.Status(), proc.Suspension())
	}
	store.fail.Store(false)
	if err := engine.Save(t.Context(), proc.ID()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(t.Context(), proc.ID())
	if err != nil || snapshot.Status != core.StatusWaiting || snapshot.Suspension == nil {
		t.Fatalf("snapshot status=%s suspension=%#v err=%v", snapshot.Status, snapshot.Suspension, err)
	}
}

func TestKillWaitingProcessPersistsTerminalSnapshot(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "snapshot-waiting-kill", ProcessStore: store, AutoSnapshot: true,
	})
	a := autoSnapshotWaitingAgent()
	proc, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil || proc.Status() != core.StatusWaiting {
		t.Fatalf("waiting run status=%s err=%v", proc.Status(), err)
	}
	if err := engine.Kill(t.Context(), proc.ID()); err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusKilled || proc.Suspension() != nil {
		t.Fatalf("killed process status=%s suspension=%#v", proc.Status(), proc.Suspension())
	}
	snapshot, err := store.Load(t.Context(), proc.ID())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != core.StatusKilled || snapshot.Suspension != nil {
		t.Fatalf("killed snapshot = %#v", snapshot)
	}
}

func TestKillWaitingProcessReportsTerminalSnapshotFailure(t *testing.T) {
	storeErr := errors.New("snapshot unavailable")
	store := newFlakyProcessStore(storeErr)
	store.fail.Store(false)
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "snapshot-waiting-kill-failure", ProcessStore: store, AutoSnapshot: true,
	})
	a := autoSnapshotWaitingAgent()
	proc, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil || proc.Status() != core.StatusWaiting {
		t.Fatalf("waiting run status=%s err=%v", proc.Status(), err)
	}
	store.fail.Store(true)
	if err := engine.Kill(t.Context(), proc.ID()); !errors.Is(err, storeErr) {
		t.Fatalf("Kill error = %v, want snapshot failure", err)
	}
	if proc.Status() != core.StatusKilled || proc.Suspension() != nil || proc.Failure() != nil {
		t.Fatalf("killed process status=%s suspension=%#v failure=%v", proc.Status(), proc.Suspension(), proc.Failure())
	}
	snapshot, err := store.Load(t.Context(), proc.ID())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != core.StatusWaiting {
		t.Fatalf("last durable snapshot = %#v, want original waiting state", snapshot)
	}
}

func TestAutoSnapshotFailurePolicyReportOnlyPublishesDegradation(t *testing.T) {
	storeErr := errors.New("snapshot unavailable")
	store := newFlakyProcessStore(storeErr)
	var degraded event.ProcessSnapshotFailed
	listener := event.NewNamedListener("snapshot-degradation", func(_ context.Context, value event.Event) {
		if failure, ok := value.(event.ProcessSnapshotFailed); ok {
			degraded = failure
		}
	})
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "snapshot-report", ProcessStore: store, AutoSnapshot: true,
		SnapshotFailurePolicy: runtime.SnapshotFailureReportOnly,
		Extensions:            []core.Extension{listener},
	})
	a := autoSnapshotAgent()
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, core.Input(word{Text: "lynx"}), core.ProcessOptions{})
	if err != nil || proc.Status() != core.StatusCompleted {
		t.Fatalf("reported run status=%s err=%v", proc.Status(), err)
	}
	if degraded.Policy != core.SnapshotFailureReportOnly || !errors.Is(degraded.Err, storeErr) {
		t.Fatalf("degradation event = %#v", degraded)
	}
	if _, err := store.Load(t.Context(), proc.ID()); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("unexpected durable snapshot: %v", err)
	}
}
