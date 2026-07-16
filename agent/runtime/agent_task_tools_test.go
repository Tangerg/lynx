package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// blockingChild builds a child agent whose single action blocks on
// release before doubling its input. It lets a test observe a
// background task in its running state, then let it finish on demand.
func blockingChild(name string, release <-chan struct{}) *core.Agent {
	return agent.New(agent.AgentConfig{Name: name, Description: "blocks until released, then doubles", Actions: []agent.Action{agent.NewAction("work", func(ctx context.Context, _ *core.ProcessContext, in subInput) (subOutput, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return subOutput{}, ctx.Err()
		}
		return subOutput{Doubled: in.Value * 2}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[subOutput](core.GoalConfig{Description: "doubled"})}})
}

// TestStartChildContinuesAfterParent pins the core async
// contract deterministically (via the done channel, no sleeps): a child
// spawned with StartChild keeps running after the parent action
// returns and the parent process completes; the result is collected
// later once the child finishes; and the child's work still counts
// toward the parent's budget subtree.
func TestStartChildContinuesAfterParent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	release := make(chan struct{})
	child := blockingChild("bg-child", release)
	childDeployment, err := engine.Deploy(child)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	// The parent action spawns the child and returns WITHOUT waiting.
	var taskID string
	var childDone <-chan error
	parent := agent.New(agent.AgentConfig{Name: "bg-parent", Actions: []agent.Action{agent.NewAction("start", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		child, done, err := engine.StartChild(ctx, childDeployment, in)
		if err != nil {
			return parentOutput{}, err
		}
		taskID, childDone = child.ID(), done
		return parentOutput{Final: 0}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "spawned"})}})
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := engine.Run(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Parent completed while the child is still blocked in the background.
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}
	if taskID == "" {
		t.Fatal("no task id captured from StartChild")
	}
	bg, ok := engine.Process(taskID)
	if !ok {
		t.Fatal("background child not registered on the engine")
	}
	if bg.Status().IsTerminal() {
		t.Fatalf("child should still be running (blocked), got %s", bg.Status())
	}

	// Release it and wait — via the done channel — for the background
	// loop to exit cleanly.
	close(release)
	if err := <-childDone; err != nil {
		t.Fatalf("background child run error: %v", err)
	}
	if bg.Status() != core.StatusCompleted {
		t.Fatalf("child status = %s; failure=%v", bg.Status(), bg.Failure())
	}
	out, ok := core.Result[subOutput](bg)
	if !ok || out.Doubled != 42 {
		t.Fatalf("child result = %+v ok=%v, want Doubled=42", out, ok)
	}

	// Budget aggregation: the background child's action rolls into the
	// parent's subtree count (parent's start + child's work).
	if _, _, actions := proc.Usage(); actions < 2 {
		t.Fatalf("parent subtree action count = %d, want >= 2 (parent + child)", actions)
	}
}

// TestTaskTools_StartRunningThenResultDone drives the
// LLM-facing tool pair: the start tool returns a task id immediately
// (status running), a result call while the child is blocked reports
// running, and a result call after completion returns the typed result
// (status done).
func TestTaskTools_StartRunningThenResultDone(t *testing.T) {
	completed := make(chan string, 4)
	listener := event.NewNamedListener("background-completed", func(_ context.Context, value event.Event) {
		if _, ok := value.(event.ProcessCompleted); ok {
			completed <- value.ProcessID()
		}
	})
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{listener}})
	release := make(chan struct{})
	child := blockingChild("bg-tool-child", release)
	_, err := engine.Deploy(child)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	startTool, resultTool, err := runtime.NewAgentTaskTools[subInput, subOutput](engine, child.Name())
	if err != nil {
		t.Fatalf("build background tools: %v", err)
	}
	if got := startTool.Definition().Name; got != "bg-tool-child_start" {
		t.Fatalf("start tool name = %q, want bg-tool-child_start", got)
	}
	if got := resultTool.Definition().Name; got != "bg-tool-child_result" {
		t.Fatalf("result tool name = %q, want bg-tool-child_result", got)
	}

	var taskID, runningResult string
	parent := agent.New(agent.AgentConfig{Name: "bg-tool-parent", Actions: []agent.Action{agent.NewAction("drive", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		args, _ := json.Marshal(in)
		var startOutput string
		startOutput, err = startTool.Call(ctx, string(args))
		if err != nil {
			return parentOutput{}, err
		}
		var sp struct {
			TaskID string `json:"task_id"`
			Status string `json:"status"`
		}
		err = json.Unmarshal([]byte(startOutput), &sp)
		if err != nil {
			return parentOutput{}, err
		}
		if sp.Status != "running" {
			t.Fatalf("start status = %q, want running", sp.Status)
		}
		taskID = sp.TaskID
		runningResult, err = resultTool.Call(ctx, `{"task_id":"`+taskID+`"}`)
		if err != nil {
			return parentOutput{}, err
		}
		return parentOutput{Final: 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "driven"})}})
	if _, err = engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := engine.Run(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}

	var running struct {
		Status string `json:"status"`
	}
	err = json.Unmarshal([]byte(runningResult), &running)
	if err != nil {
		t.Fatalf("unmarshal running result: %v", err)
	}
	if running.Status != "running" {
		t.Fatalf("result-while-blocked status = %q, want running", running.Status)
	}

	// Release and wait on the Framework's terminal event rather than polling
	// wall-clock time, then result → done with the result.
	close(release)
	for completedID := range completed {
		if completedID == taskID {
			break
		}
	}
	doneOut, err := resultTool.Call(t.Context(), `{"task_id":"`+taskID+`"}`)
	if err != nil {
		t.Fatalf("result done: %v", err)
	}
	var done struct {
		Status string    `json:"status"`
		Result subOutput `json:"result"`
	}
	if err := json.Unmarshal([]byte(doneOut), &done); err != nil {
		t.Fatalf("unmarshal done result: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("result status = %q, want done", done.Status)
	}
	if done.Result.Doubled != 42 {
		t.Fatalf("result Doubled = %d, want 42", done.Result.Doubled)
	}
}

// TestTaskTools_ResultUnknownTaskErrors verifies the result
// tool fails clearly on an id it doesn't recognize.
func TestTaskTools_ResultUnknownTaskErrors(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	child := childAgent()
	_, err := engine.Deploy(child)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	_, resultTool, err := runtime.NewAgentTaskTools[subInput, subOutput](engine, child.Name())
	if err != nil {
		t.Fatalf("build tools: %v", err)
	}
	if _, err := resultTool.Call(t.Context(), `{"task_id":"nope"}`); err == nil {
		t.Fatal("expected an error reading an unknown task id")
	}
}

// TestKillChildren_SweepsOutstandingBackgroundChildren verifies the
// turn-exit cleanup helper kills a parent's still-running background
// children and leaves their status Killed.
func TestKillChildren_SweepsOutstandingBackgroundChildren(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	release := make(chan struct{})
	defer close(release) // unblock the killed child's goroutine on exit
	child := blockingChild("kc-child", release)
	childDeployment, err := engine.Deploy(child)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	var taskID string
	parent := agent.New(agent.AgentConfig{Name: "kc-parent", Actions: []agent.Action{agent.NewAction("start", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		child, _, err := engine.StartChild(ctx, childDeployment, in)
		if err != nil {
			return parentOutput{}, err
		}
		taskID = child.ID()
		return parentOutput{Final: 0}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "spawned"})}})
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := engine.Run(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 7}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	bg, ok := engine.Process(taskID)
	if !ok {
		t.Fatal("child not registered")
	}
	if bg.Status().IsTerminal() {
		t.Fatalf("child should still be running, got %s", bg.Status())
	}

	killed := engine.KillChildren(proc.ID())
	if len(killed) != 1 || killed[0] != taskID {
		t.Fatalf("KillChildren = %v, want [%s]", killed, taskID)
	}
	if bg.Status() != core.StatusKilled {
		t.Fatalf("child status = %s, want killed", bg.Status())
	}
}
