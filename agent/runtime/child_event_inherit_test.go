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
	created   []event.ProcessCreated
	completed []event.ProcessCompleted
}

func (*pidCapture) Name() string { return "pid-capture" }

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
	case event.ProcessCompleted:
		c.completed = append(c.completed, ev)
	}
}

// TestChildEventsReachParentProcessListener verifies the runtime
// propagates a parent's process-scope EventListener onto the child
// processes it spawns: a listener registered ONLY via
// ProcessOptions.Extensions (not engine-scope) sees events from the
// subtask the parent delegates to, each carrying the child's own
// ProcessID. Before listener inheritance this listener saw the parent
// only — child events reached just the engine multicast.
func TestChildEventsReachParentProcessListener(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parent := agent.New(agent.AgentConfig{Name: "parent-observed", Description: "spawns a child while a process-scope listener watches", Actions: []agent.Action{agent.NewAction("invoke-child", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, "child-agent")
		args, _ := json.Marshal(in)
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
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	capture := &pidCapture{}
	proc, err := engine.Run(
		t.Context(), parent,
		core.Input(subInput{Value: 21}),
		// Process-scope ONLY — the listener is not on Config.
		core.ProcessOptions{Extensions: []core.Extension{capture}},
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
	// The point of the fix: a child process (id != parent) surfaced on
	// the parent's process-scope listener.
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
	childID := ""
	for _, ev := range capture.created {
		if ev.ProcessID() == proc.ID() {
			continue
		}
		value, exists := ev.Bindings.Get(core.DefaultBindingName)
		in, ok := value.(subInput)
		if !exists || !ok || in.Value != 21 {
			t.Fatalf("child ProcessCreated bindings = %#v, want subInput{21}", ev.Bindings)
		}
		childID = ev.ProcessID()
		break
	}
	if childID == "" {
		t.Fatal("no child ProcessCreated event captured")
	}
	for _, ev := range capture.completed {
		if ev.ProcessID() != childID {
			continue
		}
		out, ok := ev.Result.(subOutput)
		if !ok || out.Doubled != 42 {
			t.Fatalf("child ProcessCompleted result = %#v, want subOutput{42}", ev.Result)
		}
		return
	}
	t.Fatalf("no child ProcessCompleted event captured for %s", childID)
}
