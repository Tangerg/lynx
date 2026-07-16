package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type nestedAgentInput struct {
	Value int `json:"value"`
}

type nestedAgentOutput struct {
	Value int `json:"value"`
}

type nestedAgentStage struct {
	Value int `json:"value"`
}

type nestedParentModel struct {
	mu       sync.Mutex
	calls    int
	toolName string
}

func (m *nestedParentModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	m.calls++
	toolName := m.toolName
	m.mu.Unlock()
	if toolName == "" {
		toolName = "nested-child"
	}

	for _, message := range request.Messages {
		if message.Role == chat.RoleTool {
			return nestedTextResponse("parent complete")
		}
	}
	message := chat.NewAssistantMessage(
		chat.NewToolCallPart(chat.ToolCall{ID: "before-call", Name: "before", Arguments: `{}`}),
		chat.NewToolCallPart(chat.ToolCall{ID: "child-call", Name: toolName, Arguments: `{"value":21}`}),
	)
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
}

func (m *nestedParentModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func nestedTextResponse(text string) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func nestedChildAgent(twoSuspensions bool, completed *atomic.Int32) *core.Agent {
	if twoSuspensions {
		return agent.New(agent.AgentConfig{
			Name:        "nested-child",
			Description: "child with two durable nested suspensions",
			Actions: []agent.Action{
				agent.NewAction("first", func(ctx context.Context, _ *core.ProcessContext, input nestedAgentInput) (nestedAgentStage, error) {
					approved, err := hitl.Interrupt[bool](ctx, "nested-first", "approve first child step?")
					if err != nil {
						return nestedAgentStage{}, err
					}
					if !approved {
						return nestedAgentStage{Value: -1}, nil
					}
					return nestedAgentStage(input), nil
				}, core.ActionConfig{}),
				agent.NewAction("second", func(ctx context.Context, _ *core.ProcessContext, input nestedAgentStage) (nestedAgentOutput, error) {
					approved, err := hitl.Interrupt[bool](ctx, "nested-second", "approve second child step?")
					if err != nil {
						return nestedAgentOutput{}, err
					}
					if !approved {
						return nestedAgentOutput{Value: -2}, nil
					}
					if completed != nil {
						completed.Add(1)
					}
					return nestedAgentOutput{Value: input.Value * 2}, nil
				}, core.ActionConfig{}),
			},
			Goals: []*agent.Goal{agent.NewOutputGoal[nestedAgentOutput](core.GoalConfig{Description: "nested child complete"})},
		})
	}
	return nestedSingleSuspensionAgent("nested-child", completed)
}

func nestedSingleSuspensionAgent(name string, completed *atomic.Int32) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name:        name,
		Description: "child with durable nested suspension",
		Actions: []agent.Action{
			agent.NewAction("prepare", func(ctx context.Context, pc *core.ProcessContext, input nestedAgentInput) (nestedAgentStage, error) {
				pc.RecordModelCall(ctx, core.ModelCall{
					Model:            "nested-fixture",
					Provider:         "test",
					CostUSD:          0.25,
					PromptTokens:     2,
					CompletionTokens: 1,
				})
				return nestedAgentStage(input), nil
			}, core.ActionConfig{}),
			agent.NewAction("answer", func(ctx context.Context, _ *core.ProcessContext, input nestedAgentStage) (nestedAgentOutput, error) {
				approved, err := hitl.Interrupt[bool](ctx, "nested-first", "approve first child step?")
				if err != nil {
					return nestedAgentOutput{}, err
				}
				if !approved {
					return nestedAgentOutput{Value: -1}, nil
				}
				if completed != nil {
					completed.Add(1)
				}
				return nestedAgentOutput{Value: input.Value * 2}, nil
			}, core.ActionConfig{}),
		},
		Goals: []*agent.Goal{agent.NewOutputGoal[nestedAgentOutput](core.GoalConfig{Description: "nested child complete"})},
	})
}

func nestedFailingChildAgent(failures *atomic.Int32) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name:        "nested-child",
		Description: "child that fails after a durable nested suspension",
		Actions: []agent.Action{agent.NewAction("answer", func(ctx context.Context, _ *core.ProcessContext, _ nestedAgentInput) (nestedAgentOutput, error) {
			if _, err := hitl.Interrupt[bool](ctx, "nested-first", "approve failing child step?"); err != nil {
				return nestedAgentOutput{}, err
			}
			failures.Add(1)
			return nestedAgentOutput{}, errors.New("nested child failed after resume")
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[nestedAgentOutput](core.GoalConfig{Description: "unreachable nested child output"})},
	})
}

func nestedParentAgent(t *testing.T, engine *runtime.Engine, model *nestedParentModel, beforeCalls *atomic.Int32) *core.Agent {
	return nestedManagedParentAgent(t, engine, "nested-parent", "nested-child", model, beforeCalls)
}

func nestedManagedParentAgent(
	t *testing.T,
	engine *runtime.Engine,
	parentName string,
	childName string,
	model *nestedParentModel,
	beforeCalls *atomic.Int32,
) *core.Agent {
	t.Helper()
	childTool, err := runtime.NewAgentTool[nestedAgentInput, nestedAgentOutput](engine, childName)
	if err != nil {
		t.Fatalf("NewAgentTool: %v", err)
	}
	before, err := tools.New[struct{}, string](tools.Config{Name: "before"}, func(context.Context, struct{}) (string, error) {
		beforeCalls.Add(1)
		return "before complete", nil
	})
	if err != nil {
		t.Fatalf("before tool: %v", err)
	}
	registry, err := tools.NewRegistry(before, childTool)
	if err != nil {
		t.Fatalf("tool registry: %v", err)
	}
	return agent.New(agent.AgentConfig{
		Name: parentName,
		Actions: []agent.Action{agent.NewAction("supervise", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
			request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("delegate")))
			if err != nil {
				return "", err
			}
			result, err := pc.Interact(ctx, core.Interaction{Model: model, Request: request, Tools: registry})
			if err != nil {
				return "", err
			}
			if result.Final == nil || result.Final.Response == nil {
				return "", errors.New("parent interaction produced no final response")
			}
			return result.Final.Response.Text(), nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "nested parent complete"})},
	})
}

func nestedDelegatingAgent(t *testing.T, engine *runtime.Engine, name, childName string) *core.Agent {
	t.Helper()
	childTool, err := runtime.NewAgentTool[nestedAgentInput, nestedAgentOutput](engine, childName)
	if err != nil {
		t.Fatalf("NewAgentTool: %v", err)
	}
	return agent.New(agent.AgentConfig{
		Name:        name,
		Description: "middle agent that delegates to a suspending child",
		Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input nestedAgentInput) (nestedAgentOutput, error) {
			arguments, err := json.Marshal(input)
			if err != nil {
				return nestedAgentOutput{}, err
			}
			output, err := childTool.Call(ctx, string(arguments))
			if err != nil {
				return nestedAgentOutput{}, err
			}
			var result nestedAgentOutput
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				return nestedAgentOutput{}, err
			}
			return result, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[nestedAgentOutput](core.GoalConfig{Description: "middle delegation complete"})},
	})
}

func deployNestedTree(
	t *testing.T,
	engine *runtime.Engine,
	model *nestedParentModel,
	beforeCalls *atomic.Int32,
	completed *atomic.Int32,
) *core.Agent {
	t.Helper()
	if _, err := engine.Deploy(nestedSingleSuspensionAgent("nested-leaf", completed)); err != nil {
		t.Fatalf("deploy leaf: %v", err)
	}
	middle := nestedDelegatingAgent(t, engine, "nested-middle", "nested-leaf")
	if _, err := engine.Deploy(middle); err != nil {
		t.Fatalf("deploy middle: %v", err)
	}
	parent := nestedManagedParentAgent(t, engine, "nested-root", "nested-middle", model, beforeCalls)
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy root: %v", err)
	}
	return parent
}

func deployNestedAgents(t *testing.T, engine *runtime.Engine, twoSuspensions bool, completed *atomic.Int32, model *nestedParentModel, beforeCalls *atomic.Int32) *core.Agent {
	t.Helper()
	if _, err := engine.Deploy(nestedChildAgent(twoSuspensions, completed)); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent := nestedParentAgent(t, engine, model, beforeCalls)
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}
	return parent
}

func runNestedParent(t *testing.T, engine *runtime.Engine, parent *core.Agent) *runtime.Process {
	t.Helper()
	process, err := engine.Run(t.Context(), parent, map[string]any{core.DefaultBindingName: struct{}{}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run parent: %v", err)
	}
	return process
}

func nestedChildProcess(t *testing.T, engine *runtime.Engine, parentID string) *runtime.Process {
	t.Helper()
	var child *runtime.Process
	for _, candidate := range engine.Processes() {
		if candidate.ParentID() != parentID {
			continue
		}
		if child != nil {
			t.Fatalf("parent %q has multiple children %q and %q", parentID, child.ID(), candidate.ID())
		}
		child = candidate
	}
	if child == nil {
		t.Fatalf("parent %q has no child process", parentID)
	}
	return child
}

func directNestedParentAgent(t *testing.T, engine *runtime.Engine, prepared *atomic.Int32) *core.Agent {
	t.Helper()
	childTool, err := runtime.NewAgentTool[nestedAgentInput, nestedAgentOutput](engine, "nested-child")
	if err != nil {
		t.Fatalf("NewAgentTool: %v", err)
	}
	return agent.New(agent.AgentConfig{
		Name: "direct-nested-parent",
		Actions: []agent.Action{
			agent.NewAction("prepare", func(context.Context, *core.ProcessContext, struct{}) (nestedAgentInput, error) {
				prepared.Add(1)
				return nestedAgentInput{Value: 21}, nil
			}, core.ActionConfig{}),
			agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input nestedAgentInput) (nestedAgentOutput, error) {
				arguments, err := json.Marshal(input)
				if err != nil {
					return nestedAgentOutput{}, err
				}
				output, err := childTool.Call(ctx, string(arguments))
				if err != nil {
					return nestedAgentOutput{}, err
				}
				var result nestedAgentOutput
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					return nestedAgentOutput{}, err
				}
				return result, nil
			}, core.ActionConfig{}),
		},
		Goals: []*agent.Goal{agent.NewOutputGoal[nestedAgentOutput](core.GoalConfig{Description: "direct nested parent complete"})},
	})
}

func TestAgentToolNestedSuspensionParksParentAndResumesOriginalToolCall(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	var beforeCalls atomic.Int32
	var childCompletions atomic.Int32
	model := &nestedParentModel{}
	parent := deployNestedAgents(t, engine, false, &childCompletions, model, &beforeCalls)

	process := runNestedParent(t, engine, parent)
	if process.Status() != core.StatusWaiting {
		t.Fatalf("parent status = %s, want waiting; failure=%v", process.Status(), process.Failure())
	}
	suspension := process.Suspension()
	if suspension == nil || suspension.ID != "nested-first" || suspension.Kind != interaction.SuspensionHuman {
		t.Fatalf("parent suspension = %#v, want nested child human suspension", suspension)
	}
	child := nestedChildProcess(t, engine, process.ID())
	if child.Status() != core.StatusWaiting {
		t.Fatalf("child status = %s, want waiting", child.Status())
	}
	if beforeCalls.Load() != 1 || model.Calls() != 1 {
		t.Fatalf("before/model calls = %d/%d, want 1/1 before resume", beforeCalls.Load(), model.Calls())
	}

	if err := engine.Resume(process.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume parent: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue parent: %v", err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s, want completed; failure=%v", process.Status(), process.Failure())
	}
	if beforeCalls.Load() != 1 || model.Calls() != 2 || childCompletions.Load() != 1 {
		t.Fatalf("before/model/child completions = %d/%d/%d, want 1/2/1", beforeCalls.Load(), model.Calls(), childCompletions.Load())
	}
	if _, ok := engine.Process(child.ID()); ok {
		t.Fatalf("completed nested child %q remained registered", child.ID())
	}
}

func TestAgentToolDirectNestedSuspensionResumesOriginalChild(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	var prepared atomic.Int32
	var childCompletions atomic.Int32
	if _, err := engine.Deploy(nestedChildAgent(false, &childCompletions)); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent := directNestedParentAgent(t, engine, &prepared)
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	process, err := engine.Run(
		t.Context(),
		parent,
		map[string]any{core.DefaultBindingName: struct{}{}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run parent: %v", err)
	}
	if process.Status() != core.StatusWaiting || process.Suspension() == nil || process.Suspension().ID != "nested-first" {
		t.Fatalf("parent status/suspension = %s/%#v", process.Status(), process.Suspension())
	}
	child := nestedChildProcess(t, engine, process.ID())

	if err := engine.Resume(process.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume parent: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue parent: %v", err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s, want completed; failure=%v", process.Status(), process.Failure())
	}
	if prepared.Load() != 1 || childCompletions.Load() != 1 {
		t.Fatalf("parent prepare/child completions = %d/%d, want 1/1", prepared.Load(), childCompletions.Load())
	}
	if _, ok := engine.Process(child.ID()); ok {
		t.Fatalf("completed direct nested child %q remained registered", child.ID())
	}
}

func TestAgentToolNestedSuspensionReusesChildAcrossConsecutivePauses(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	var beforeCalls atomic.Int32
	var childCompletions atomic.Int32
	model := &nestedParentModel{}
	parent := deployNestedAgents(t, engine, true, &childCompletions, model, &beforeCalls)

	process := runNestedParent(t, engine, parent)
	child := nestedChildProcess(t, engine, process.ID())
	if err := engine.Resume(process.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume first: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue first: %v", err)
	}
	if process.Status() != core.StatusWaiting || process.Suspension() == nil || process.Suspension().ID != "nested-second" {
		t.Fatalf("after first resume parent status/suspension = %s/%#v", process.Status(), process.Suspension())
	}
	if current := nestedChildProcess(t, engine, process.ID()); current.ID() != child.ID() {
		t.Fatalf("second pause created child %q, want original %q", current.ID(), child.ID())
	}
	if beforeCalls.Load() != 1 || model.Calls() != 1 {
		t.Fatalf("before/model calls after first resume = %d/%d, want 1/1", beforeCalls.Load(), model.Calls())
	}

	if err := engine.Resume(process.ID(), "nested-second", true); err != nil {
		t.Fatalf("Resume second: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue second: %v", err)
	}
	if process.Status() != core.StatusCompleted || childCompletions.Load() != 1 {
		t.Fatalf("final parent status/child completions = %s/%d; failure=%v", process.Status(), childCompletions.Load(), process.Failure())
	}
	if beforeCalls.Load() != 1 || model.Calls() != 2 {
		t.Fatalf("before/model calls = %d/%d, want 1/2", beforeCalls.Load(), model.Calls())
	}
}

func TestAgentToolNestedSuspensionRestoresProcessTreeWithoutReplay(t *testing.T) {
	store := core.NewMemoryProcessStore()
	var beforeCalls atomic.Int32
	var childCompletions atomic.Int32
	model1 := &nestedParentModel{}
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: "nested-restore", ProcessStore: store, AutoSnapshot: true})
	parent1 := deployNestedAgents(t, engine1, false, &childCompletions, model1, &beforeCalls)

	process1 := runNestedParent(t, engine1, parent1)
	child1 := nestedChildProcess(t, engine1, process1.ID())
	if process1.Status() != core.StatusWaiting || child1.Status() != core.StatusWaiting {
		t.Fatalf("stored tree statuses parent=%s child=%s", process1.Status(), child1.Status())
	}

	model2 := &nestedParentModel{}
	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "nested-restore", ProcessStore: store, AutoSnapshot: true})
	deployNestedAgents(t, engine2, false, &childCompletions, model2, &beforeCalls)
	restored, err := engine2.RestoreResumable(t.Context(), process1.ID(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreResumable parent: %v", err)
	}
	restoredChild, ok := engine2.Process(child1.ID())
	if !ok || restoredChild.ParentID() != restored.ID() || restoredChild.Status() != core.StatusWaiting {
		t.Fatalf("restored child = %#v found=%v", restoredChild, ok)
	}
	if cost, tokens, _ := restored.Usage(); cost != 0.25 || tokens != 3 || len(restored.ModelCalls()) != 2 {
		t.Fatalf("restored parent usage = cost %.2f tokens %d calls %d, want 0.25/3/2", cost, tokens, len(restored.ModelCalls()))
	}
	if cost, tokens, _ := restoredChild.Usage(); cost != 0.25 || tokens != 3 || len(restoredChild.ModelCalls()) != 1 {
		t.Fatalf("restored child usage = cost %.2f tokens %d calls %d, want 0.25/3/1", cost, tokens, len(restoredChild.ModelCalls()))
	}

	if err := engine2.Resume(restored.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume restored parent: %v", err)
	}
	if err := engine2.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("Continue restored parent: %v", err)
	}
	if restored.Status() != core.StatusCompleted {
		t.Fatalf("restored parent status = %s; failure=%v", restored.Status(), restored.Failure())
	}
	if beforeCalls.Load() != 1 || model1.Calls() != 1 || model2.Calls() != 1 || childCompletions.Load() != 1 {
		t.Fatalf("before/model1/model2/child = %d/%d/%d/%d, want 1/1/1/1", beforeCalls.Load(), model1.Calls(), model2.Calls(), childCompletions.Load())
	}
	if cost, tokens, _ := restored.Usage(); cost != 0.25 || tokens != 3 || len(restored.ModelCalls()) != 3 {
		t.Fatalf("completed restored usage = cost %.2f tokens %d calls %d, want 0.25/3/3", cost, tokens, len(restored.ModelCalls()))
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(ids) != 1 || ids[0] != restored.ID() {
		t.Fatalf("stored process ids = %v, want only parent %q", ids, restored.ID())
	}
}

func TestAgentToolNestedSuspensionRestoresMultiLevelProcessTree(t *testing.T) {
	store := core.NewMemoryProcessStore()
	sessionStore := core.NewMemorySessionStore()
	var beforeCalls atomic.Int32
	var leafCompletions atomic.Int32
	model1 := &nestedParentModel{toolName: "nested-middle"}
	engine1 := agent.MustNewEngine(runtime.Config{
		BuildID:      "nested-multi-level",
		ProcessStore: store,
		SessionStore: sessionStore,
		AutoSnapshot: true,
	})
	parent1 := deployNestedTree(t, engine1, model1, &beforeCalls, &leafCompletions)

	rootSession := core.NewSession("nested-root-conversation", "nested-user", parent1.Name())
	root1, err := engine1.RunInSession(
		t.Context(),
		parent1,
		&rootSession,
		map[string]any{core.DefaultBindingName: struct{}{}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunInSession root: %v", err)
	}
	middle1 := nestedChildProcess(t, engine1, root1.ID())
	leaf1 := nestedChildProcess(t, engine1, middle1.ID())
	if root1.Status() != core.StatusWaiting || middle1.Status() != core.StatusWaiting || leaf1.Status() != core.StatusWaiting {
		t.Fatalf("stored tree statuses root/middle/leaf = %s/%s/%s", root1.Status(), middle1.Status(), leaf1.Status())
	}
	storedMiddleSession, err := sessionStore.Load(t.Context(), middle1.ID())
	if err != nil {
		t.Fatalf("load original middle session: %v", err)
	}
	storedMiddleSession.Metadata["restore_marker"] = "preserved"
	if err := sessionStore.Save(t.Context(), storedMiddleSession); err != nil {
		t.Fatalf("save marked middle session: %v", err)
	}
	if err := sessionStore.Delete(t.Context(), leaf1.ID()); err != nil {
		t.Fatalf("delete leaf session: %v", err)
	}

	model2 := &nestedParentModel{toolName: "nested-middle"}
	engine2 := agent.MustNewEngine(runtime.Config{
		BuildID:      "nested-multi-level",
		ProcessStore: store,
		SessionStore: sessionStore,
		AutoSnapshot: true,
	})
	deployNestedTree(t, engine2, model2, &beforeCalls, &leafCompletions)
	restored, err := engine2.RestoreResumable(t.Context(), root1.ID(), core.ProcessOptions{Session: &rootSession})
	if err != nil {
		t.Fatalf("RestoreResumable root: %v", err)
	}
	restoredMiddle, ok := engine2.Process(middle1.ID())
	if !ok || restoredMiddle.ParentID() != restored.ID() || restoredMiddle.Status() != core.StatusWaiting {
		t.Fatalf("restored middle = %#v found=%v", restoredMiddle, ok)
	}
	restoredLeaf, ok := engine2.Process(leaf1.ID())
	if !ok || restoredLeaf.ParentID() != restoredMiddle.ID() || restoredLeaf.Status() != core.StatusWaiting {
		t.Fatalf("restored leaf = %#v found=%v", restoredLeaf, ok)
	}
	middleSession, err := sessionStore.Load(t.Context(), restoredMiddle.ID())
	if err != nil {
		t.Fatalf("load restored middle session: %v", err)
	}
	if middleSession.ParentID != rootSession.ID {
		t.Fatalf("restored middle session parent = %q, want %q", middleSession.ParentID, rootSession.ID)
	}
	if middleSession.Metadata["restore_marker"] != "preserved" {
		t.Fatalf("restored middle session metadata = %v, want preserved marker", middleSession.Metadata)
	}
	leafSession, err := sessionStore.Load(t.Context(), restoredLeaf.ID())
	if err != nil {
		t.Fatalf("load restored leaf session: %v", err)
	}
	if leafSession.ParentID != restoredMiddle.ID() {
		t.Fatalf("restored leaf session parent = %q, want %q", leafSession.ParentID, restoredMiddle.ID())
	}
	if cost, tokens, _ := restored.Usage(); cost != 0.25 || tokens != 3 || len(restored.ModelCalls()) != 2 {
		t.Fatalf("restored multi-level usage = cost %.2f tokens %d calls %d, want 0.25/3/2", cost, tokens, len(restored.ModelCalls()))
	}

	if err := engine2.Resume(restored.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume restored root: %v", err)
	}
	if err := engine2.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("Continue restored root: %v", err)
	}
	if restored.Status() != core.StatusCompleted {
		t.Fatalf("restored root status = %s; failure=%v", restored.Status(), restored.Failure())
	}
	if beforeCalls.Load() != 1 || model1.Calls() != 1 || model2.Calls() != 1 || leafCompletions.Load() != 1 {
		t.Fatalf("before/model1/model2/leaf = %d/%d/%d/%d, want 1/1/1/1", beforeCalls.Load(), model1.Calls(), model2.Calls(), leafCompletions.Load())
	}
	if _, ok := engine2.Process(middle1.ID()); ok {
		t.Fatalf("completed middle %q remained registered", middle1.ID())
	}
	if _, ok := engine2.Process(leaf1.ID()); ok {
		t.Fatalf("completed leaf %q remained registered", leaf1.ID())
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(ids) != 1 || ids[0] != restored.ID() {
		t.Fatalf("stored process ids = %v, want only root %q", ids, restored.ID())
	}
}

func TestAgentToolNestedSuspensionMissingChildSnapshotIsLost(t *testing.T) {
	store := core.NewMemoryProcessStore()
	var beforeCalls atomic.Int32
	model1 := &nestedParentModel{}
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: "nested-missing", ProcessStore: store, AutoSnapshot: true})
	parent1 := deployNestedAgents(t, engine1, false, nil, model1, &beforeCalls)
	process1 := runNestedParent(t, engine1, parent1)
	child1 := nestedChildProcess(t, engine1, process1.ID())
	if err := store.Delete(t.Context(), child1.ID()); err != nil {
		t.Fatalf("delete child snapshot: %v", err)
	}

	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "nested-missing", ProcessStore: store, AutoSnapshot: true})
	deployNestedAgents(t, engine2, false, nil, &nestedParentModel{}, &beforeCalls)
	resumable, err := engine2.Resumable(t.Context(), process1.ID())
	if err != nil {
		t.Fatalf("Resumable: %v", err)
	}
	if resumable {
		t.Fatal("Resumable accepted parent whose nested child snapshot is missing")
	}
	if _, err := engine2.RestoreResumable(t.Context(), process1.ID(), core.ProcessOptions{}); !errors.Is(err, runtime.ErrResumableSnapshotLost) {
		t.Fatalf("RestoreResumable error = %v, want ErrResumableSnapshotLost", err)
	}
}

func TestAgentToolNestedSuspensionRestoreRollbackPreservesReplacedTerminalProcess(t *testing.T) {
	store := core.NewMemoryProcessStore()
	var beforeCalls atomic.Int32
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: "nested-rollback", ProcessStore: store, AutoSnapshot: true})
	parent1 := deployNestedAgents(t, engine1, false, nil, &nestedParentModel{}, &beforeCalls)
	process1 := runNestedParent(t, engine1, parent1)

	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "nested-rollback", ProcessStore: store, AutoSnapshot: true})
	deployNestedAgents(t, engine2, false, nil, &nestedParentModel{}, &beforeCalls)
	terminalSnapshot, err := store.Load(t.Context(), process1.ID())
	if err != nil {
		t.Fatalf("load parent snapshot: %v", err)
	}
	terminalSnapshot.Status = core.StatusCompleted
	terminalSnapshot.Suspension = nil
	previous, err := engine2.RestoreSnapshot(terminalSnapshot, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreSnapshot terminal predecessor: %v", err)
	}

	_, err = engine2.RestoreResumable(t.Context(), process1.ID(), core.ProcessOptions{
		ChildOptions: func(context.Context, core.ProcessView, *core.Agent) (core.ProcessOptions, error) {
			return core.ProcessOptions{}, errors.New("restore child options unavailable")
		},
	})
	if !errors.Is(err, runtime.ErrResumableSnapshotLost) {
		t.Fatalf("RestoreResumable error = %v, want ErrResumableSnapshotLost", err)
	}
	current, ok := engine2.Process(process1.ID())
	if !ok || current != previous || current.Status() != core.StatusCompleted {
		t.Fatalf("rollback current process = %#v found=%v, want original terminal process", current, ok)
	}
}

func TestAgentToolNestedSuspensionRestoresTerminalChildCrashWindow(t *testing.T) {
	store := core.NewMemoryProcessStore()
	var beforeCalls atomic.Int32
	var childCompletions atomic.Int32
	model1 := &nestedParentModel{}
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: "nested-terminal-window", ProcessStore: store, AutoSnapshot: true})
	parent1 := deployNestedAgents(t, engine1, false, &childCompletions, model1, &beforeCalls)
	process1 := runNestedParent(t, engine1, parent1)
	child1 := nestedChildProcess(t, engine1, process1.ID())

	if err := engine1.Resume(process1.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume parent: %v", err)
	}
	if err := engine1.Continue(t.Context(), child1.ID()); err != nil {
		t.Fatalf("Continue child directly: %v", err)
	}
	if child1.Status() != core.StatusCompleted || childCompletions.Load() != 1 {
		t.Fatalf("child terminal window status/completions = %s/%d", child1.Status(), childCompletions.Load())
	}

	model2 := &nestedParentModel{}
	engine2 := agent.MustNewEngine(runtime.Config{BuildID: "nested-terminal-window", ProcessStore: store, AutoSnapshot: true})
	deployNestedAgents(t, engine2, false, &childCompletions, model2, &beforeCalls)
	restored, err := engine2.RestoreResumable(t.Context(), process1.ID(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreResumable: %v", err)
	}
	restoredChild, ok := engine2.Process(child1.ID())
	if !ok || restoredChild.Status() != core.StatusCompleted {
		t.Fatalf("restored terminal child = %#v found=%v", restoredChild, ok)
	}
	if err := engine2.Resume(restored.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume restored parent: %v", err)
	}
	if err := engine2.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("Continue restored parent: %v", err)
	}
	if restored.Status() != core.StatusCompleted || childCompletions.Load() != 1 {
		t.Fatalf("parent status/child completions = %s/%d; failure=%v", restored.Status(), childCompletions.Load(), restored.Failure())
	}
	if beforeCalls.Load() != 1 || model2.Calls() != 1 {
		t.Fatalf("before/model2 calls = %d/%d, want 1/1", beforeCalls.Load(), model2.Calls())
	}
}

func TestAgentToolNestedSuspensionManualSaveCleansRemovedTerminalChildSnapshot(t *testing.T) {
	store := core.NewMemoryProcessStore()
	var beforeCalls atomic.Int32
	engine := agent.MustNewEngine(runtime.Config{BuildID: "nested-manual-cleanup", ProcessStore: store})
	parent := deployNestedAgents(t, engine, false, nil, &nestedParentModel{}, &beforeCalls)
	process := runNestedParent(t, engine, parent)
	child := nestedChildProcess(t, engine, process.ID())
	if _, err := engine.Save(t.Context(), process.ID()); err != nil {
		t.Fatalf("save waiting process tree: %v", err)
	}

	if err := engine.Resume(process.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume parent: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue parent: %v", err)
	}
	if process.Status() != core.StatusCompleted || child.Status() != core.StatusCompleted {
		t.Fatalf("parent/child status = %s/%s, want completed/completed", process.Status(), child.Status())
	}
	if err := engine.Remove(child.ID()); err != nil {
		t.Fatalf("Remove terminal child: %v", err)
	}
	if _, err := engine.Save(t.Context(), process.ID()); err != nil {
		t.Fatalf("save completed parent: %v", err)
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(ids) != 1 || ids[0] != process.ID() {
		t.Fatalf("stored process ids = %v, want only parent %q", ids, process.ID())
	}
}

func TestAgentToolNestedSuspensionCleansKilledChild(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	var beforeCalls atomic.Int32
	model := &nestedParentModel{}
	parent := deployNestedAgents(t, engine, false, nil, model, &beforeCalls)
	process := runNestedParent(t, engine, parent)
	child := nestedChildProcess(t, engine, process.ID())

	if err := engine.Kill(child.ID()); err != nil {
		t.Fatalf("Kill child: %v", err)
	}
	if err := engine.Resume(process.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume parent: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue parent: %v", err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s, want completed; failure=%v", process.Status(), process.Failure())
	}
	if beforeCalls.Load() != 1 || model.Calls() != 2 {
		t.Fatalf("before/model calls = %d/%d, want 1/2", beforeCalls.Load(), model.Calls())
	}
	if _, ok := engine.Process(child.ID()); ok {
		t.Fatalf("killed nested child %q remained registered", child.ID())
	}
}

func TestAgentToolNestedSuspensionCleansFailedChild(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	var beforeCalls atomic.Int32
	var failures atomic.Int32
	if _, err := engine.Deploy(nestedFailingChildAgent(&failures)); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	model := &nestedParentModel{}
	parent := nestedParentAgent(t, engine, model, &beforeCalls)
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}
	process := runNestedParent(t, engine, parent)
	child := nestedChildProcess(t, engine, process.ID())

	if err := engine.Resume(process.ID(), "nested-first", true); err != nil {
		t.Fatalf("Resume parent: %v", err)
	}
	if err := engine.Continue(t.Context(), process.ID()); err != nil {
		t.Fatalf("Continue parent: %v", err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s, want completed; failure=%v", process.Status(), process.Failure())
	}
	if failures.Load() != 1 || beforeCalls.Load() != 1 || model.Calls() != 2 {
		t.Fatalf("child failures/before/model calls = %d/%d/%d, want 1/1/2", failures.Load(), beforeCalls.Load(), model.Calls())
	}
	if _, ok := engine.Process(child.ID()); ok {
		t.Fatalf("failed nested child %q remained registered", child.ID())
	}
}

func TestAgentToolNestedSuspensionKillParentTerminatesChild(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	var beforeCalls atomic.Int32
	parent := deployNestedAgents(t, engine, false, nil, &nestedParentModel{}, &beforeCalls)
	process := runNestedParent(t, engine, parent)
	child := nestedChildProcess(t, engine, process.ID())

	if err := engine.Kill(process.ID()); err != nil {
		t.Fatalf("Kill parent: %v", err)
	}
	if process.Status() != core.StatusKilled || child.Status() != core.StatusKilled {
		t.Fatalf("parent/child status = %s/%s, want killed/killed", process.Status(), child.Status())
	}
}

func TestStandaloneAgentToolKeepsExternalWaitingResult(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(nestedChildAgent(false, nil)); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	tool, err := runtime.NewStandaloneAgentTool[nestedAgentInput, nestedAgentOutput](engine, "nested-child")
	if err != nil {
		t.Fatalf("NewStandaloneAgentTool: %v", err)
	}
	output, err := tool.Call(t.Context(), `{"value":21}`)
	if err != nil {
		t.Fatalf("standalone Call: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode waiting result: %v", err)
	}
	if payload["status"] != "waiting" || payload["suspension_id"] != "nested-first" {
		t.Fatalf("standalone waiting payload = %v", payload)
	}
}
