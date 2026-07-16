package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// failingSessionStore fails after child creation reaches session persistence.
type failingSessionStore struct{}

func (failingSessionStore) Save(context.Context, core.Session) error {
	return errors.New("session store unavailable")
}
func (failingSessionStore) Load(context.Context, string) (core.Session, error) {
	return core.Session{}, core.ErrSessionNotFound
}
func (failingSessionStore) Delete(context.Context, string) error   { return nil }
func (failingSessionStore) List(context.Context) ([]string, error) { return nil, nil }

// TestRunChildRollsBackOnSessionLinkFailure verifies that session persistence
// failure unregisters the half-created child.
func TestRunChildRollsBackOnSessionLinkFailure(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{SessionStore: failingSessionStore{}})

	// A completed parent provides a registered process for direct delegation.
	parentDef := agent.New(agent.AgentConfig{Name: "parent", Actions: []agent.Action{agent.NewAction("noop", func(_ context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		return parentOutput{Final: in.Value}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	if _, err := engine.Deploy(parentDef); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}
	parent, err := engine.Run(t.Context(), parentDef,
		map[string]any{core.DefaultBindingName: subInput{Value: 1}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run parent: %v", err)
	}

	before := len(engine.Processes())
	childDeployment, err := engine.Deploy(childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	// Session linking fails after registration; creation must roll back.
	ctx := core.WithProcessView(t.Context(), parent)
	if _, err := engine.RunChild(ctx, childDeployment, subInput{Value: 21}); err == nil {
		t.Fatal("RunChild returned nil error after session persistence failed")
	}

	if after := len(engine.Processes()); after != before {
		t.Errorf("registry grew %d → %d — the half-created child leaked instead of being unregistered", before, after)
	}
}
