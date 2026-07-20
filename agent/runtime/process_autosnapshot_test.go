package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

type flakyProcessStore struct {
	inner *core.MemoryProcessStore
	fail  atomic.Bool
	err   error
}

func newFlakyProcessStore(err error) *flakyProcessStore {
	store := &flakyProcessStore{inner: core.NewMemoryProcessStore(), err: err}
	store.fail.Store(true)
	return store
}

func (s *flakyProcessStore) Save(ctx context.Context, snapshot core.ProcessSnapshot) error {
	if s.fail.Load() {
		return s.err
	}
	return s.inner.Save(ctx, snapshot)
}

func (s *flakyProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	return s.inner.Load(ctx, id)
}

func autoSnapshotAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "auto-snapshot-policy", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})
}

// TestAutoSnapshot_PersistsTerminalState verifies that with AutoSnapshot on
// and a ProcessStore configured, a completed run is persisted automatically
// (no explicit Save call) and is loadable afterward.
func TestAutoSnapshot_PersistsTerminalState(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "snap", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	store := core.NewMemoryProcessStore()
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

	store := core.NewMemoryProcessStore()
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
	if err != nil || loaded.Revision != 1 || loaded.Status != core.StatusCompleted {
		t.Fatalf("loaded = %#v, err %v", loaded, err)
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
