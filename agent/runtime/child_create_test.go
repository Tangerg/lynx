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

var errChildSessionUnavailable = errors.New("session store unavailable")

func (failingSessionStore) Save(context.Context, core.Session) error {
	return errChildSessionUnavailable
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
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parentDef := agent.New(agent.AgentConfig{Name: "parent", Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
		_, err := engine.RunChild(ctx, childDeployment, input)
		return parentOutput{}, err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	before := len(engine.Processes())
	createdBefore := created.value()
	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run parent: %v", err)
	}
	if parent.Status() != core.StatusFailed || !errors.Is(parent.Failure(), errChildSessionUnavailable) {
		t.Fatalf("parent status=%s failure=%v", parent.Status(), parent.Failure())
	}

	if after := len(engine.Processes()); after != before+1 {
		t.Errorf("registry grew %d → %d, want only the parent", before, after)
	}
	if after := created.value(); after != createdBefore+1 {
		t.Errorf("ProcessCreated count grew %d → %d, want only the parent event", createdBefore, after)
	}
}

func TestRunChildReturnsExtensionNamePanic(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parentDef := agent.New(agent.AgentConfig{Name: "parent-extension-check", Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
		_, err := engine.RunChild(ctx, childDeployment, input)
		return parentOutput{}, err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	cause := errors.New("child extension identity unavailable")
	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{
		ChildOptions: func(context.Context, core.ProcessView, *core.Agent) (core.ProcessOptions, error) {
			return core.ProcessOptions{Extensions: []core.Extension{panickingChildExtension{cause: cause}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}
	if parent.Status() != core.StatusFailed || !errors.Is(parent.Failure(), cause) {
		t.Fatalf("parent status=%s failure=%v, want extension panic", parent.Status(), parent.Failure())
	}
}

func TestRunChildRejectsInactiveParent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatal(err)
	}
	parentDef := agent.New(agent.AgentConfig{Name: "inactive-parent", Actions: []agent.Action{agent.NewAction("finish", func(_ context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
		return parentOutput{Final: input.Value}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before := len(engine.Processes())
	_, err = engine.RunChild(core.WithProcessView(t.Context(), parent), childDeployment, subInput{Value: 2})
	if !errors.Is(err, runtime.ErrChildParentInactive) {
		t.Fatalf("RunChild error = %v, want ErrChildParentInactive", err)
	}
	if got := len(engine.Processes()); got != before {
		t.Fatalf("registered process count = %d, want %d", got, before)
	}
}
