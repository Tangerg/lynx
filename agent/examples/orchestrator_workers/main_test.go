package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/goap"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func TestRun(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	const want = "objective: ship agent\n" +
		"workers: facts, risks, recommendation\n" +
		"summary: synthesized 3 ordered worker results\n" +
		"processes: 6\n"
	if output.String() != want {
		t.Fatalf("output=%q, want %q", output.String(), want)
	}
}

func TestInteractionCanDelegateExactPlanningWorkers(t *testing.T) {
	planningWorker, taskState := newPlanningWorker(t)
	budget, err := agent.NewBudget(32, 32, 64)
	if err != nil {
		t.Fatal(err)
	}
	delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name:        "review_with_planning",
		Description: "Use goal-directed planning to complete one review task.",
		Deployment:  planningWorker,
		Budget:      budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &planningDelegateModel{}
	validator := func(candidate interaction.CompletionCandidate) (interaction.CompletionDecision, error) {
		artifacts := candidate.Artifacts().All()
		if len(artifacts) != 2 {
			return interaction.CompletionDecision{Feedback: "Complete both planned review tasks."}, nil
		}
		for index, artifact := range artifacts {
			output, err := interaction.DecodeArtifact[planning.Output](artifact)
			if err != nil {
				return interaction.CompletionDecision{}, err
			}
			if artifact.DelegateName() != "review_with_planning" ||
				output.Outcome != planning.OutcomeAchieved {
				return interaction.CompletionDecision{}, fmt.Errorf("planning artifact %d was not achieved", index)
			}
		}
		return interaction.CompletionDecision{Accepted: true}, nil
	}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:        "example.orchestrator_workers.planning_delegate",
		Description: "Delegate model-selected tasks to exact Planning workers.",
		Version:     "1.0.0", MaxModelCalls: 2,
		Delegates: []interaction.Delegate{delegate}, CompletionValidator: validator,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	root, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("planning-delegate-root-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("planning-delegate-root-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver: deploymentResolver{planningWorker.DeploymentRef(): planningWorker},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("close Engine: %v", err)
		}
	})
	input, err := agent.EncodeInput(interaction.Input{Messages: []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("plan both reviews, then synthesize")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Start(context.Background(), root, input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	output, err := decodeCompleted[interaction.Output](result)
	if err != nil {
		t.Fatal(err)
	}
	if output.ModelResponse == nil || output.ModelResponse.Text() != "planning workers achieved 2 tasks" ||
		model.Calls() != 2 || taskState.CompletedCount() != 2 {
		t.Fatalf("output=%#v model calls=%d completed=%d", output, model.Calls(), taskState.CompletedCount())
	}
	tree, err := engine.CaptureTree(context.Background(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshots := tree.ProcessSnapshots()
	if len(snapshots) != 3 {
		t.Fatalf("planning Delegate tree contains %d Processes, want 3", len(snapshots))
	}
	planningChildren := 0
	for _, snapshot := range snapshots {
		if snapshot.DeploymentRef() == planningWorker.DeploymentRef() {
			planningChildren++
		}
	}
	if planningChildren != 2 {
		t.Fatalf("planning Delegate tree contains %d exact Planning children, want 2", planningChildren)
	}
}

type planningTaskState struct {
	mu        sync.Mutex
	completed map[string]bool
}

func (state *planningTaskState) CompletedCount() int {
	state.mu.Lock()
	defer state.mu.Unlock()
	return len(state.completed)
}

func newPlanningWorker(t *testing.T) (agent.Deployment, *planningTaskState) {
	t.Helper()
	inputSchema, err := agent.SchemaFor[workerTask]()
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err := planning.NewCondition("review.complete", planning.False)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := planning.NewCondition("review.complete", planning.True)
	if err != nil {
		t.Fatal(err)
	}
	action, err := planning.NewAction(planning.ActionConfig{
		Name: "review", Description: "Complete the requested review task.",
		Preconditions: []planning.Condition{incomplete}, Effects: []planning.Condition{complete},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := planning.NewDispatcherBinding(planning.DispatcherBindingConfig{Action: action})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := planning.NewGoal(planning.GoalConfig{
		Name: "review_complete", Description: "The requested review task is complete.",
		Conditions: []planning.Condition{complete},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := planning.NewDefinition(planning.DefinitionConfig{
		Name:        "example.orchestrator_workers.planning_worker",
		Description: "Use GOAP to complete one model-selected review task.",
		Version:     "1.0.0", InputSchema: inputSchema, Goal: goal,
		Actions: []planning.ActionBinding{binding}, Planner: goap.New(goap.Config{}),
		MaxActionAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	state := &planningTaskState{completed: make(map[string]bool)}
	observer := planning.ObserverFunc(func(
		_ context.Context,
		request planning.ObservationRequest,
	) (planning.WorldState, error) {
		task, err := agent.DecodeInput[workerTask](request.Input)
		if err != nil {
			return planning.WorldState{}, err
		}
		state.mu.Lock()
		done := state.completed[task.ID]
		state.mu.Unlock()
		condition := incomplete
		if done {
			condition = complete
		}
		return planning.NewWorldState(condition)
	})
	executor := planning.ActionExecutorFunc(func(
		_ context.Context,
		request planning.ActionRequest,
	) (planning.ActionResult, error) {
		if request.ActionName != "review" {
			return planning.ActionResult{}, errors.New("unexpected planning action")
		}
		task, err := agent.DecodeInput[workerTask](request.Input)
		if err != nil {
			return planning.ActionResult{}, err
		}
		state.mu.Lock()
		state.completed[task.ID] = true
		state.mu.Unlock()
		return planning.ActionSucceeded(), nil
	})
	dispatcher, err := planning.NewDispatcher(definition, planning.DispatcherConfig{
		Observer: observer, ActionExecutors: map[string]planning.ActionExecutor{"review": executor},
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("planning-worker-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("planning-worker-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment, state
}

type planningDelegateModel struct {
	mu    sync.Mutex
	calls int
}

func (model *planningDelegateModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	if model.calls == 1 {
		if len(request.Tools) != 1 || request.Tools[0].Name != "review_with_planning" {
			return nil, fmt.Errorf("planning Delegate manifest=%#v", request.Tools)
		}
		first, err := json.Marshal(workerTask{
			ID: "facts", Objective: "ship agent", Instruction: "review facts",
		})
		if err != nil {
			return nil, fmt.Errorf("encode first Planning task: %w", err)
		}
		second, err := json.Marshal(workerTask{
			ID: "risks", Objective: "ship agent", Instruction: "review risks",
		})
		if err != nil {
			return nil, fmt.Errorf("encode second Planning task: %w", err)
		}
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{
				ID: "call_planning_facts", Name: "review_with_planning", Arguments: string(first),
			}),
			chat.NewToolCallPart(chat.ToolCall{
				ID: "call_planning_risks", Name: "review_with_planning", Arguments: string(second),
			}),
		)
		return chat.NewResponse(&chat.Result{
			Message: &message, FinishReason: chat.FinishReasonToolCalls,
		}, nil)
	}
	if len(request.Messages) != 3 || request.Messages[2].Role != chat.RoleTool ||
		len(request.Messages[2].Parts) != 2 {
		return nil, fmt.Errorf("planning results context=%#v", request.Messages)
	}
	for index, part := range request.Messages[2].Parts {
		if part.ToolResult == nil || part.ToolResult.IsError {
			return nil, fmt.Errorf("planning tool result %d=%#v", index, part.ToolResult)
		}
		var output planning.Output
		if err := json.Unmarshal([]byte(part.ToolResult.Result), &output); err != nil ||
			output.Outcome != planning.OutcomeAchieved {
			return nil, fmt.Errorf("planning tool result %d did not achieve goal: %#v, %v", index, output, err)
		}
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("planning workers achieved 2 tasks"))
	return chat.NewResponse(&chat.Result{
		Message: &message, FinishReason: chat.FinishReasonStop,
	}, nil)
}

func (model *planningDelegateModel) Calls() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.calls
}
