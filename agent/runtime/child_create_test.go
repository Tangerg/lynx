package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
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

type processCreatedCounter struct {
	mu    sync.Mutex
	count int
}

type panickingChildExtension struct{ cause error }

func (e panickingChildExtension) Name() string { panic(e.cause) }

func (*processCreatedCounter) Name() string { return "process-created-counter" }

func (c *processCreatedCounter) OnEvent(_ context.Context, value event.Event) {
	if _, ok := value.(event.ProcessCreated); !ok {
		return
	}
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

func (c *processCreatedCounter) value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// TestRunChildRollsBackOnSessionLinkFailure verifies that session persistence
// failure unregisters the half-created child.
func TestRunChildRollsBackOnSessionLinkFailure(t *testing.T) {
	created := &processCreatedCounter{}
	engine := agent.MustNewEngine(runtime.Config{
		ChildSessionStore: failingSessionStore{},
		Extensions:        []core.Extension{created},
	})

	// A completed parent provides a registered process for direct delegation.
	parentDef := agent.New(agent.AgentConfig{Name: "parent", Actions: []agent.Action{agent.NewAction("noop", func(_ context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		return parentOutput{Final: in.Value}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	if _, err := engine.Deploy(parentDef); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}
	parent, err := engine.Run(t.Context(), parentDef,
		core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run parent: %v", err)
	}

	before := len(engine.Processes())
	createdBefore := created.value()
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
	if after := created.value(); after != createdBefore {
		t.Errorf("ProcessCreated count grew %d → %d for a rolled-back child", createdBefore, after)
	}
}

func TestRunChildReturnsExtensionNamePanic(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parentDef := agent.New(agent.AgentConfig{Name: "parent-extension-check", Actions: []agent.Action{agent.NewAction("noop", func(_ context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		return parentOutput{Final: in.Value}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	if _, err := engine.Deploy(parentDef); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}
	cause := errors.New("child extension identity unavailable")
	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{
		ChildOptions: func(context.Context, core.ProcessView, *core.Agent) (core.ProcessOptions, error) {
			return core.ProcessOptions{Extensions: []core.Extension{panickingChildExtension{cause: cause}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}

	ctx := core.WithProcessView(t.Context(), parent)
	if _, err := engine.RunChild(ctx, childDeployment, subInput{Value: 21}); !errors.Is(err, cause) {
		t.Fatalf("RunChild error = %v, want wrapped extension Name panic", err)
	}
}
