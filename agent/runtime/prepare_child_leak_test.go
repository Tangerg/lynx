package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// failingSessionStore makes child session linking fail at Save time so a child
// spawn errors AFTER CreateChildProcess has already registered the child.
type failingSessionStore struct{}

func (failingSessionStore) Save(context.Context, core.Session) error {
	return errors.New("session store unavailable")
}
func (failingSessionStore) Load(context.Context, string) (core.Session, error) {
	return core.Session{}, core.ErrSessionNotFound
}
func (failingSessionStore) Delete(context.Context, string) error   { return nil }
func (failingSessionStore) List(context.Context) ([]string, error) { return nil, nil }

// TestSpawnChild_UnregistersOnLinkSessionFailure pins the cleanup of a
// half-created child: session linking runs AFTER CreateChildProcess has
// already registered the child, so a failure must unregister the child — left
// behind it leaks at StatusNotStarted, which
// PruneTerminalProcesses never reaps. Spawning directly (not via a planner
// loop) keeps the failure path isolated. (subInput/parentOutput/childAgent
// live in subagent_test.go, same package.)
func TestSpawnChild_UnregistersOnLinkSessionFailure(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{SessionStore: failingSessionStore{}})

	// A trivial parent that completes in one tick — gives a registered parent
	// process to spawn from, without driving a planner loop.
	parentDef := agent.New("parent").
		Actions(agent.NewAction("noop",
			func(_ context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				return parentOutput{Final: in.Value}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "done"})).
		Build()
	if err := platform.Deploy(parentDef); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}
	parent, err := platform.RunAgent(t.Context(), parentDef,
		map[string]any{core.DefaultBindingName: subInput{Value: 1}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent parent: %v", err)
	}

	before := len(platform.ActiveProcesses())

	// Spawn a child directly: session linking fails at Save, so the child
	// CreateChildProcess just registered must be unregistered, not leaked.
	ctx := core.WithProcess(t.Context(), parent)
	if _, err := runtime.SpawnChildProtectedOnly(ctx, platform, childAgent(), subInput{Value: 21}); err == nil {
		t.Fatal("SpawnChildProtectedOnly should fail when the session store rejects the link")
	}

	if after := len(platform.ActiveProcesses()); after != before {
		t.Errorf("registry grew %d → %d — the half-created child leaked instead of being unregistered", before, after)
	}
}
