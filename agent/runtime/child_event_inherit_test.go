package runtime_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// pidCapture counts events per emitting ProcessID. Used to assert a
// process-scope listener also receives the events of child processes
// spawned during the run (each tagged with the child's own id).
type pidCapture struct {
	mu        sync.Mutex
	ids       map[string]int
	parents   map[string]string
	created   []event.ProcessCreated
	completed []event.ProcessCompleted
}

func (*pidCapture) Name() string    { return "pid-capture" }
func (*pidCapture) ObserveSubtree() {}

func (c *pidCapture) OnEvent(_ context.Context, e event.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = map[string]int{}
	}
	c.ids[e.ProcessID()]++
	switch ev := e.(type) {
	case event.ProcessCreated:
		c.created = append(c.created, ev)
		if c.parents == nil {
			c.parents = map[string]string{}
		}
		c.parents[ev.ProcessID()] = ev.ParentID
	case event.ProcessCompleted:
		c.completed = append(c.completed, ev)
	}
}

// TestChildEventsReachParentProcessListener verifies that a parent's
// process-scope SubtreeEventListener receives events from spawned child
// processes, each carrying the child's own ProcessID. The listener is
// registered only through ProcessOptions.Extensions, not at engine scope.
func TestChildEventsReachParentProcessListener(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parent := agent.New(agent.AgentConfig{Name: "parent-observed", Description: "spawns a child while a process-scope listener watches", Actions: []agent.Action{agent.NewAction("invoke-child", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		tool, err := runtime.NewAgentTool[subInput, subOutput](engine, childDeployment)
		if err != nil {
			return parentOutput{}, err
		}
		args, err := json.Marshal(in)
		if err != nil {
			return parentOutput{}, err
		}
		out, err := tool.Call(ctx, string(args))
		if err != nil {
			return parentOutput{}, err
		}
		var decoded subOutput
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			return parentOutput{}, err
		}
		return parentOutput{Final: decoded.Doubled}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "final produced"})}})
	if _, err := engine.Deploy(t.Context(), parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	capture := &pidCapture{}
	localIDs := make(map[string]int)
	local := runtime.NewEventListener("local-capture", func(_ context.Context, published event.Event) {
		localIDs[published.ProcessID()]++
	})
	proc, err := engine.Run(
		t.Context(), parent,
		core.Input(subInput{Value: 21}),
		// Process-scope ONLY — the listener is not on Config.
		core.ProcessOptions{Extensions: []core.Extension{capture, local}},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.ids[proc.ID()] == 0 {
		t.Fatalf("listener saw no events for the parent process %s", proc.ID())
	}
	// A child process (id != parent) must surface on the parent's
	// process-scope listener.
	sawChild := false
	for id := range capture.ids {
		if id != proc.ID() {
			sawChild = true
			break
		}
	}
	if !sawChild {
		t.Fatalf("process-scope listener saw only the parent (%v); expected child events too", capture.ids)
	}
	if len(localIDs) != 1 || localIDs[proc.ID()] == 0 {
		t.Fatalf("process-local listener crossed into child scope: %v", localIDs)
	}
	childID := ""
	for _, ev := range capture.created {
		if ev.ProcessID() == proc.ID() {
			continue
		}
		// The event states lineage, not the child's input: a host that wants the
		// input reads it from the process this names.
		if ev.ParentID != proc.ID() {
			t.Fatalf("child ProcessCreated ParentID = %q, want %q", ev.ParentID, proc.ID())
		}
		if ev.SpawnCallID == "" {
			t.Fatal("AgentTool child ProcessCreated omitted SpawnCallID")
		}
		childID = ev.ProcessID()
		break
	}
	if childID == "" {
		t.Fatal("no child ProcessCreated event captured")
	}
	if got := capture.parents[childID]; got != proc.ID() {
		t.Fatalf("child ParentID during ProcessCreated = %q, want %q", got, proc.ID())
	}
	for _, ev := range capture.completed {
		if ev.ProcessID() != childID {
			continue
		}
		if ev.Goal.Name() == "" {
			t.Fatal("child ProcessCompleted omitted its goal descriptor")
		}
		child, ok := engine.Process(childID)
		if !ok {
			t.Fatalf("completed child process %s is not queryable", childID)
		}
		out, ok := core.Result[subOutput](child)
		if !ok || out.Doubled != 42 {
			t.Fatalf("child process result = %#v, want subOutput{42}", out)
		}
		return
	}
	t.Fatalf("no child ProcessCompleted event captured for %s", childID)
}
