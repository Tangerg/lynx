package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

type childAdmissionGate struct {
	name     string
	entered  chan core.ProcessView
	release  <-chan struct{}
	rejected error
	panicErr error
	calls    atomic.Int32
}

func (g *childAdmissionGate) Name() string { return g.name }

func (g *childAdmissionGate) AdmitChild(ctx context.Context, child core.ProcessView) error {
	g.calls.Add(1)
	if g.panicErr != nil {
		panic(g.panicErr)
	}
	if g.entered != nil {
		select {
		case g.entered <- child:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if g.release != nil {
		select {
		case <-g.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return g.rejected
}

type childRunResult struct {
	process *runtime.Process
	err     error
}

func TestChildAdmissionCompletesBeforeCreatedEventAndExecution(t *testing.T) {
	release := make(chan struct{})
	gate := &childAdmissionGate{
		name:    "blocking-child-admission",
		entered: make(chan core.ProcessView, 1),
		release: release,
	}
	var childCreated atomic.Bool
	listener := runtime.NewEventListener("child-created", func(_ context.Context, published event.Event) {
		created, ok := published.(event.ProcessCreated)
		if ok && created.ParentID != "" {
			childCreated.Store(true)
		}
	})
	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{gate, listener},
	})

	var childExecuted atomic.Bool
	child := childAdmissionTestAgent("admitted-child", &childExecuted)
	childDeployment, err := engine.Deploy(t.Context(), child)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent := childAdmissionToolParent(engine, childDeployment)

	completed := make(chan childRunResult, 1)
	go func() {
		process, runErr := engine.Run(
			t.Context(),
			parent,
			core.Input(subInput{Value: 21}),
			core.ProcessOptions{},
		)
		completed <- childRunResult{process: process, err: runErr}
	}()

	var admitted core.ProcessView
	select {
	case admitted = <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("child admission was not requested")
	}
	if admitted.ID() == "" || admitted.ParentID() == "" || admitted.SpawnCallID() == "" || admitted.StartedAt().IsZero() {
		t.Fatalf(
			"admission child identity = {id:%q parent:%q spawn_call:%q started:%v}, want complete identity",
			admitted.ID(),
			admitted.ParentID(),
			admitted.SpawnCallID(),
			admitted.StartedAt(),
		)
	}
	if childCreated.Load() {
		t.Fatal("ProcessCreated was published before child admission completed")
	}
	if childExecuted.Load() {
		t.Fatal("child executed before child admission completed")
	}
	select {
	case result := <-completed:
		t.Fatalf("parent completed before child admission: process=%v error=%v", result.process, result.err)
	default:
	}

	close(release)
	select {
	case result := <-completed:
		if result.err != nil {
			t.Fatalf("run parent: %v", result.err)
		}
		if result.process.Status() != core.StatusCompleted {
			t.Fatalf("parent status = %s, want completed; failure=%v", result.process.Status(), result.process.Failure())
		}
	case <-time.After(time.Second):
		t.Fatal("parent did not complete after child admission")
	}
	if !childCreated.Load() {
		t.Fatal("admitted child did not publish ProcessCreated")
	}
	if !childExecuted.Load() {
		t.Fatal("admitted child did not execute")
	}
}

func TestRejectedChildAdmissionRemovesUnpublishedProcess(t *testing.T) {
	rejection := errors.New("child opening transaction rejected")
	gate := &childAdmissionGate{
		name:     "rejecting-child-admission",
		entered:  make(chan core.ProcessView, 1),
		rejected: rejection,
	}
	var childCreated atomic.Bool
	listener := runtime.NewEventListener("rejected-child-created", func(_ context.Context, published event.Event) {
		created, ok := published.(event.ProcessCreated)
		if ok && created.ParentID != "" {
			childCreated.Store(true)
		}
	})
	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{gate, listener},
	})

	var childExecuted atomic.Bool
	childDeployment, err := engine.Deploy(
		t.Context(),
		childAdmissionTestAgent("rejected-child", &childExecuted),
	)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent, err := engine.Run(
		t.Context(),
		childAdmissionParent(engine, childDeployment),
		core.Input(subInput{Value: 21}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}
	if parent.Status() != core.StatusFailed || !errors.Is(parent.Failure(), rejection) {
		t.Fatalf("parent status=%s failure=%v, want child admission rejection", parent.Status(), parent.Failure())
	}
	admitted := <-gate.entered
	if _, exists := engine.Process(admitted.ID()); exists {
		t.Fatalf("rejected child %q remained in the process registry", admitted.ID())
	}
	if processes := engine.Processes(); len(processes) != 1 || processes[0] != parent {
		t.Fatalf("registered processes = %#v, want only parent %q", processes, parent.ID())
	}
	if childCreated.Load() {
		t.Fatal("rejected child published ProcessCreated")
	}
	if childExecuted.Load() {
		t.Fatal("rejected child executed")
	}
}

func TestPanickingChildAdmissionRemovesUnpublishedProcess(t *testing.T) {
	panicCause := errors.New("child admission panic")
	gate := &childAdmissionGate{
		name:     "panicking-child-admission",
		entered:  make(chan core.ProcessView, 1),
		panicErr: panicCause,
	}
	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{gate},
	})

	var childExecuted atomic.Bool
	childDeployment, err := engine.Deploy(
		t.Context(),
		childAdmissionTestAgent("panic-rejected-child", &childExecuted),
	)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent, err := engine.Run(
		t.Context(),
		childAdmissionParent(engine, childDeployment),
		core.Input(subInput{Value: 21}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}
	if parent.Status() != core.StatusFailed || !errors.Is(parent.Failure(), panicCause) {
		t.Fatalf("parent status=%s failure=%v, want child admission panic", parent.Status(), parent.Failure())
	}
	if processes := engine.Processes(); len(processes) != 1 || processes[0] != parent {
		t.Fatalf("registered processes = %#v, want only parent %q", processes, parent.ID())
	}
	if childExecuted.Load() {
		t.Fatal("child with panicking admission executed")
	}
}

func TestProcessChildAdmitterOverridesEngineFallback(t *testing.T) {
	engineRejection := errors.New("engine fallback must not run")
	engineGate := &childAdmissionGate{
		name:     "engine-child-admission",
		rejected: engineRejection,
	}
	processGate := &childAdmissionGate{name: "process-child-admission"}
	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{engineGate},
	})

	var childExecuted atomic.Bool
	childDeployment, err := engine.Deploy(
		t.Context(),
		childAdmissionTestAgent("process-admitted-child", &childExecuted),
	)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent, err := engine.Run(
		t.Context(),
		childAdmissionParent(engine, childDeployment),
		core.Input(subInput{Value: 21}),
		core.ProcessOptions{
			ChildOptions: func(context.Context, core.ProcessView, core.AgentDescriptor) (core.ProcessOptions, error) {
				return core.ProcessOptions{Extensions: []core.Extension{processGate}}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}
	if parent.Status() != core.StatusCompleted {
		t.Fatalf("parent status=%s failure=%v, want completed", parent.Status(), parent.Failure())
	}
	if got := processGate.calls.Load(); got != 1 {
		t.Fatalf("process child-admitter calls = %d, want 1", got)
	}
	if got := engineGate.calls.Load(); got != 0 {
		t.Fatalf("engine child-admitter calls = %d, want 0", got)
	}
	if !childExecuted.Load() {
		t.Fatal("process-admitted child did not execute")
	}
}

func childAdmissionTestAgent(name string, executed *atomic.Bool) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: name,
		Actions: []agent.Action{agent.NewAction(
			"complete-child",
			func(_ context.Context, _ *core.ProcessContext, input subInput) (subOutput, error) {
				executed.Store(true)
				return subOutput{Doubled: input.Value * 2}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{agent.NewOutputGoal[subOutput](core.GoalConfig{Description: "child completed"})},
	})
}

func childAdmissionParent(engine *runtime.Engine, child *runtime.Deployment) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: "child-admission-parent",
		Actions: []agent.Action{agent.NewAction(
			"run-child",
			func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
				process, err := engine.RunChild(ctx, child, input)
				if err != nil {
					return parentOutput{}, err
				}
				output, ok := core.Result[subOutput](process)
				if !ok {
					return parentOutput{}, errors.New("child result is missing")
				}
				return parentOutput{Final: output.Doubled}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "child completed"})},
	})
}

func childAdmissionToolParent(engine *runtime.Engine, child *runtime.Deployment) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: "child-admission-tool-parent",
		Actions: []agent.Action{agent.NewAction(
			"run-child-tool",
			func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
				tool, err := runtime.NewAgentTool[subInput, subOutput](engine, child)
				if err != nil {
					return parentOutput{}, err
				}
				arguments, err := json.Marshal(input)
				if err != nil {
					return parentOutput{}, err
				}
				rawOutput, err := tool.Call(ctx, string(arguments))
				if err != nil {
					return parentOutput{}, err
				}
				var output subOutput
				if err := json.Unmarshal([]byte(rawOutput), &output); err != nil {
					return parentOutput{}, err
				}
				return parentOutput{Final: output.Doubled}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "child completed"})},
	})
}
