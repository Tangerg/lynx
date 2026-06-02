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
	mu  sync.Mutex
	ids map[string]int
}

func (*pidCapture) Name() string { return "pid-capture" }

func (c *pidCapture) OnEvent(e event.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = map[string]int{}
	}
	c.ids[e.ProcessID()]++
}

// TestChildEventsReachParentProcessListener verifies the runtime
// propagates a parent's process-scope EventListener onto the child
// processes it spawns: a listener registered ONLY via
// ProcessOptions.Extensions (not platform-scope) sees events from the
// subtask the parent delegates to, each carrying the child's own
// ProcessID. Before listener inheritance this listener saw the parent
// only — child events reached just the platform multicast.
func TestChildEventsReachParentProcessListener(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parent := agent.New("parent-observed").
		Description("spawns a child while a process-scope listener watches").
		Actions(agent.NewAction("invoke-child",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				tool, _ := runtime.AsChatTool[subInput, subOutput](platform, "child-agent")
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
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "final produced"})).
		Build()
	if err := platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	capture := &pidCapture{}
	proc, err := platform.RunAgent(
		t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}},
		// Process-scope ONLY — the listener is not on PlatformConfig.
		core.ProcessOptions{Extensions: []core.Extension{capture}},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
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
}
