package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// blockingChild builds a child agent whose single action blocks on
// release before doubling its input. It lets a test observe a
// background task in its running state, then let it finish on demand.
func blockingChild(name string, release <-chan struct{}) *core.Agent {
	return agent.New(name).
		Description("blocks until released, then doubles").
		Actions(agent.NewAction("work",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (subOutput, error) {
				select {
				case <-release:
				case <-ctx.Done():
					return subOutput{}, ctx.Err()
				}
				return subOutput{Doubled: in.Value * 2}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[subOutput](core.Goal{Description: "doubled"})).
		Build()
}

// waitTerminal polls until the task reaches a terminal status or the
// bounded deadline elapses. Used by the tool round-trip test, which —
// unlike SpawnChildAsync's done channel — only has the task id to work
// with. The deterministic timing guarantee lives in
// TestSpawnChildAsync_CollectAfterParentContinues.
func waitTerminal(t *testing.T, p *runtime.Platform, id string) {
	t.Helper()
	for range 2000 {
		proc, ok := p.ProcessByID(id)
		if !ok {
			t.Fatalf("task %q vanished from the registry", id)
		}
		if proc.Status().IsTerminal() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("task %q never reached a terminal state", id)
}

// TestSpawnChildAsync_CollectAfterParentContinues pins the core async
// contract deterministically (via the done channel, no sleeps): a child
// spawned with SpawnChildAsync keeps running after the parent action
// returns and the parent process completes; the result is collected
// later once the child finishes; and the child's work still counts
// toward the parent's budget subtree.
func TestSpawnChildAsync_CollectAfterParentContinues(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	release := make(chan struct{})
	child := blockingChild("bg-child", release)
	if err := platform.Deploy(child); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	// The parent action spawns the child and returns WITHOUT waiting.
	var taskID string
	var childDone <-chan error
	parent := agent.New("bg-parent").
		Actions(agent.NewAction("spawn",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				id, done, err := runtime.SpawnChildAsync(ctx, platform, child, in)
				if err != nil {
					return parentOutput{}, err
				}
				taskID, childDone = id, done
				return parentOutput{Final: 0}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "spawned"})).
		Build()
	if err := platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := platform.RunAgent(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	// Parent completed while the child is still blocked in the background.
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}
	if taskID == "" {
		t.Fatal("no task id captured from SpawnChildAsync")
	}
	bg, ok := platform.ProcessByID(taskID)
	if !ok {
		t.Fatal("background child not registered on the platform")
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
	out, ok := core.ResultOfType[subOutput](bg)
	if !ok || out.Doubled != 42 {
		t.Fatalf("child result = %+v ok=%v, want Doubled=42", out, ok)
	}

	// Budget aggregation: the background child's action rolls into the
	// parent's subtree count (parent's spawn + child's work).
	if _, _, actions := proc.Usage(); actions < 2 {
		t.Fatalf("parent subtree action count = %d, want >= 2 (parent + child)", actions)
	}
}

// TestAsBackgroundChatTool_SpawnRunningThenCollectDone drives the
// LLM-facing tool pair: the spawn tool returns a task id immediately
// (status running), a collect call while the child is blocked reports
// running, and a collect call after completion returns the typed result
// (status done).
func TestAsBackgroundChatTool_SpawnRunningThenCollectDone(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	release := make(chan struct{})
	child := blockingChild("bg-tool-child", release)
	if err := platform.Deploy(child); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	spawnTool, collectTool, err := runtime.AsBackgroundChatTool[subInput, subOutput](platform, child)
	if err != nil {
		t.Fatalf("build background tools: %v", err)
	}
	if got := spawnTool.Definition().Name; got != "bg-tool-child_spawn" {
		t.Fatalf("spawn tool name = %q, want bg-tool-child_spawn", got)
	}
	if got := collectTool.Definition().Name; got != "bg-tool-child_collect" {
		t.Fatalf("collect tool name = %q, want bg-tool-child_collect", got)
	}

	var taskID, runningResult string
	parent := agent.New("bg-tool-parent").
		Actions(agent.NewAction("drive",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				args, _ := json.Marshal(in)
				var spawnOut string
				spawnOut, err = spawnTool.Call(ctx, string(args))
				if err != nil {
					return parentOutput{}, err
				}
				var sp struct {
					TaskID string `json:"task_id"`
					Status string `json:"status"`
				}
				err = json.Unmarshal([]byte(spawnOut), &sp)
				if err != nil {
					return parentOutput{}, err
				}
				if sp.Status != "running" {
					t.Fatalf("spawn status = %q, want running", sp.Status)
				}
				taskID = sp.TaskID
				// Child is blocked → collect reports running.
				runningResult, err = collectTool.Call(ctx, `{"task_id":"`+taskID+`"}`)
				if err != nil {
					return parentOutput{}, err
				}
				return parentOutput{Final: 1}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "driven"})).
		Build()
	if err = platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := platform.RunAgent(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
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
		t.Fatalf("collect-while-blocked status = %q, want running", running.Status)
	}

	// Release, wait for terminal, then collect → done with the result.
	close(release)
	waitTerminal(t, platform, taskID)
	doneOut, err := collectTool.Call(t.Context(), `{"task_id":"`+taskID+`"}`)
	if err != nil {
		t.Fatalf("collect done: %v", err)
	}
	var done struct {
		Status string    `json:"status"`
		Result subOutput `json:"result"`
	}
	if err := json.Unmarshal([]byte(doneOut), &done); err != nil {
		t.Fatalf("unmarshal done result: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("collect status = %q, want done", done.Status)
	}
	if done.Result.Doubled != 42 {
		t.Fatalf("collected result Doubled = %d, want 42", done.Result.Doubled)
	}
}

// TestAsBackgroundChatTool_CollectUnknownTaskErrors verifies the collect
// tool fails clearly on an id it doesn't recognize.
func TestAsBackgroundChatTool_CollectUnknownTaskErrors(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	child := childAgent()
	if err := platform.Deploy(child); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	_, collectTool, err := runtime.AsBackgroundChatTool[subInput, subOutput](platform, child)
	if err != nil {
		t.Fatalf("build tools: %v", err)
	}
	if _, err := collectTool.Call(t.Context(), `{"task_id":"nope"}`); err == nil {
		t.Fatal("expected an error collecting an unknown task id")
	}
}

// TestKillChildren_SweepsOutstandingBackgroundChildren verifies the
// turn-exit cleanup helper kills a parent's still-running background
// children and leaves their status Killed.
func TestKillChildren_SweepsOutstandingBackgroundChildren(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	release := make(chan struct{})
	defer close(release) // unblock the killed child's goroutine on exit
	child := blockingChild("kc-child", release)
	if err := platform.Deploy(child); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	var taskID string
	parent := agent.New("kc-parent").
		Actions(agent.NewAction("spawn",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				id, _, err := runtime.SpawnChildAsync(ctx, platform, child, in)
				if err != nil {
					return parentOutput{}, err
				}
				taskID = id
				return parentOutput{Final: 0}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "spawned"})).
		Build()
	if err := platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := platform.RunAgent(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 7}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	bg, ok := platform.ProcessByID(taskID)
	if !ok {
		t.Fatal("child not registered")
	}
	if bg.Status().IsTerminal() {
		t.Fatalf("child should still be running, got %s", bg.Status())
	}

	killed := platform.KillChildren(proc.ID())
	if len(killed) != 1 || killed[0] != taskID {
		t.Fatalf("KillChildren = %v, want [%s]", killed, taskID)
	}
	if bg.Status() != core.StatusKilled {
		t.Fatalf("child status = %s, want killed", bg.Status())
	}
}
