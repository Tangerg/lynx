package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools"
)

func TestWaitingSubtreeCancellationPlanHasNoSideEffects(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	parent, _ := deployNestedRestoreAgents(t, engine)
	root := startWaitingNestedRestoreTree(t, engine, parent)
	before, err := engine.SnapshotTree(t.Context(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	childID := onlyChildSnapshot(t, before, root.ID()).ID

	plan, err := engine.PlanWaitingSubtreeCancellation(t.Context(), root.ID(), childID)
	if err != nil {
		t.Fatalf("PlanWaitingSubtreeCancellation: %v", err)
	}
	if got := plan.CanceledProcessIDs(); len(got) != 1 || got[0] != childID {
		t.Fatalf("canceled process IDs = %v, want [%s]", got, childID)
	}
	replacement := plan.ResultingTree()
	if len(replacement.Snapshots) != 1 {
		t.Fatalf("replacement snapshot count = %d, want 1", len(replacement.Snapshots))
	}
	beforeRoot := snapshotByID(t, before, root.ID())
	replacementRoot := snapshotByID(t, replacement, root.ID())
	childSnapshot := snapshotByID(t, before, childID)
	if replacementRoot.OwnUsage != beforeRoot.OwnUsage {
		t.Fatalf("replacement own usage = %#v, want unchanged %#v", replacementRoot.OwnUsage, beforeRoot.OwnUsage)
	}
	if replacementRoot.RetiredChildUsage != childSnapshot.OwnUsage {
		t.Fatalf(
			"replacement retired child usage = %#v, want canceled child usage %#v",
			replacementRoot.RetiredChildUsage,
			childSnapshot.OwnUsage,
		)
	}
	if pending := plan.PendingSuspensions(); len(pending) != 0 {
		t.Fatalf("replacement pending suspensions = %#v, want none", pending)
	}
	beforeUsage, err := before.Usage()
	if err != nil {
		t.Fatal(err)
	}
	replacementUsage, err := replacement.Usage()
	if err != nil {
		t.Fatal(err)
	}
	if replacementUsage != beforeUsage {
		t.Fatalf("replacement usage = %#v, want preserved %#v", replacementUsage, beforeUsage)
	}

	// Returned protocol values are copies, not aliases into the plan.
	replacement.Snapshots[0].Suspension.FrameworkState[0] = '['
	canceled := plan.CanceledProcessIDs()
	canceled[0] = "mutated"
	if again := plan.ResultingTree(); again.Snapshots[0].Suspension.FrameworkState[0] == '[' {
		t.Fatal("ResultingTree returned mutable plan state")
	}
	if again := plan.CanceledProcessIDs(); again[0] != childID {
		t.Fatal("CanceledProcessIDs returned mutable prepared state")
	}

	after, err := engine.SnapshotTree(t.Context(), root.ID())
	if err != nil {
		t.Fatalf("SnapshotTree after planning: %v", err)
	}
	if len(after.Snapshots) != len(before.Snapshots) {
		t.Fatalf("snapshot count after planning = %d, want %d", len(after.Snapshots), len(before.Snapshots))
	}
	if _, ok := engine.Process(childID); !ok {
		t.Fatalf("child %q disappeared after Abort", childID)
	}
	if err := engine.Respond(t.Context(), root.ID(), "nested-approval", true); err != nil {
		t.Fatalf("Resume after planning: %v", err)
	}
	if err := engine.ApplyWaitingSubtreeCancellation(t.Context(), plan); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("Apply changed plan error = %v, want ErrSuspensionStale", err)
	}
	if _, ok := engine.Process(childID); !ok {
		t.Fatalf("stale plan removed child %q", childID)
	}
	if err := engine.Continue(t.Context(), root.ID()); err != nil {
		t.Fatalf("Continue after planning: %v", err)
	}
	if root.Status() != core.StatusCompleted {
		t.Fatalf("root status after normal continuation = %s, want completed", root.Status())
	}
}

func TestWaitingSubtreeCancellationPlanAppliesPortableCheckpoint(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	parent, parentRef := deployNestedRestoreAgents(t, engine)
	root := startWaitingNestedRestoreTree(t, engine, parent)
	before, err := engine.SnapshotTree(t.Context(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	childID := onlyChildSnapshot(t, before, root.ID()).ID
	beforeUsage := root.Usage()

	plan, err := engine.PlanWaitingSubtreeCancellation(t.Context(), root.ID(), childID)
	if err != nil {
		t.Fatalf("PlanWaitingSubtreeCancellation: %v", err)
	}
	portable := plan.ResultingTree()
	if err := engine.ApplyWaitingSubtreeCancellation(t.Context(), plan); err != nil {
		t.Fatalf("ApplyWaitingSubtreeCancellation: %v", err)
	}
	if err := engine.ApplyWaitingSubtreeCancellation(t.Context(), plan); err == nil {
		t.Fatal("second ApplyWaitingSubtreeCancellation unexpectedly succeeded")
	}

	if _, ok := engine.Process(childID); ok {
		t.Fatalf("canceled child %q remains registered", childID)
	}
	if root.Status() != core.StatusWaiting {
		t.Fatalf("root status after apply = %s, want waiting", root.Status())
	}
	if got := root.Usage(); got != beforeUsage {
		t.Fatalf("root usage after child detach = %#v, want %#v", got, beforeUsage)
	}
	live, err := engine.SnapshotTree(t.Context(), root.ID())
	if err != nil {
		t.Fatalf("SnapshotTree after Commit: %v", err)
	}
	if len(live.Snapshots) != 1 {
		t.Fatalf("live snapshot count = %d, want 1", len(live.Snapshots))
	}
	if err := engine.Respond(t.Context(), root.ID(), "nested-approval", true); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("Resume framework-ready checkpoint error = %v, want ErrSuspensionStale", err)
	}
	if err := engine.Continue(t.Context(), root.ID()); err != nil {
		t.Fatalf("Continue settled direct checkpoint: %v", err)
	}
	if root.Status() != core.StatusFailed || !errors.Is(root.Failure(), runtime.ErrChildProcessCanceled) {
		t.Fatalf("root status/failure = %s, %v; want failed ErrChildProcessCanceled", root.Status(), root.Failure())
	}

	restoredEngine := agent.MustNewEngine(runtime.Config{})
	_, restoredParentRef := deployNestedRestoreAgents(t, restoredEngine)
	if restoredParentRef != parentRef {
		t.Fatalf("restored parent deployment = %s, want %s", restoredParentRef, parentRef)
	}
	restored, err := restoredEngine.RestoreTree(t.Context(), portable, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreTree prepared checkpoint: %v", err)
	}
	if err := restoredEngine.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("Continue restored settled checkpoint: %v", err)
	}
	if restored.Status() != core.StatusFailed || !errors.Is(restored.Failure(), runtime.ErrChildProcessCanceled) {
		t.Fatalf("restored status/failure = %s, %v; want failed ErrChildProcessCanceled", restored.Status(), restored.Failure())
	}
}

func TestWaitingSubtreeCancellationPlanSettlesManagedToolCallInOrder(t *testing.T) {
	for _, test := range []struct {
		name        string
		targetIndex int
	}{
		{name: "active call", targetIndex: 0},
		{name: "later call", targetIndex: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &waitingCancellationModel{}
			engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
			registry := managedChildRegistry(t, engine)
			parent := managedInteractionAgent(
				t,
				"waiting-cancellation-parent",
				registry,
				interaction.Limits{MaxConcurrentToolCalls: 2},
			)
			mustDeploy(t, engine, parent)

			runHandle, err := engine.Start(t.Context(), parent, managedInput(), core.ProcessOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if completion := awaitRun(t, runHandle); completion.Error() != nil {
				t.Fatal(completion.Error())
			}
			root := runHandle.Process()
			pending, err := engine.PendingSuspensions(t.Context(), root.ID())
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 2 {
				t.Fatalf("initial pending suspensions = %#v, want 2", pending)
			}
			target := pending[test.targetIndex]
			survivor := pending[1-test.targetIndex]

			plan, err := engine.PlanWaitingSubtreeCancellation(
				t.Context(),
				root.ID(),
				target.ProcessID,
			)
			if err != nil {
				t.Fatalf("PlanWaitingSubtreeCancellation: %v", err)
			}
			if got := plan.PendingSuspensions(); len(got) != 1 ||
				got[0].ProcessID != survivor.ProcessID ||
				got[0].SuspensionID != survivor.SuspensionID {
				t.Fatalf("planned pending suspensions = %#v, want survivor %#v", got, survivor)
			}
			if err := engine.ApplyWaitingSubtreeCancellation(t.Context(), plan); err != nil {
				t.Fatalf("ApplyWaitingSubtreeCancellation: %v", err)
			}

			if test.targetIndex == 0 {
				// The root has a framework-ready result before the surviving
				// external boundary. Advancing it publishes no user Resume and
				// parks on the second call without invoking either child again.
				if err := engine.Respond(
					t.Context(),
					root.ID(),
					target.SuspensionID,
					true,
				); !errors.Is(err, interaction.ErrSuspensionStale) {
					t.Fatalf("Resume framework-ready active call error = %v, want ErrSuspensionStale", err)
				}
				if err := engine.Continue(t.Context(), root.ID()); err != nil {
					t.Fatalf("Continue host-settled active call: %v", err)
				}
				if root.Status() != core.StatusWaiting {
					t.Fatalf("root status after internal continuation = %s, want waiting", root.Status())
				}
			} else if err := engine.Continue(t.Context(), root.ID()); !errors.Is(err, interaction.ErrSuspensionStale) {
				t.Fatalf("Continue with unanswered active sibling error = %v, want ErrSuspensionStale", err)
			}

			if err := engine.Respond(
				t.Context(),
				root.ID(),
				survivor.SuspensionID,
				true,
			); err != nil {
				t.Fatalf("Resume survivor: %v", err)
			}
			if err := engine.Continue(t.Context(), root.ID()); err != nil {
				t.Fatalf("Continue survivor: %v", err)
			}
			if root.Status() != core.StatusCompleted {
				t.Fatalf("root status = %s, failure=%v; want completed", root.Status(), root.Failure())
			}
			if calls := model.Calls(); calls != 2 {
				t.Fatalf("model calls = %d, want initial + continuation", calls)
			}
		})
	}
}

func TestWaitingSubtreeCancellationPlanPropagatesReadinessToManagedAncestor(t *testing.T) {
	model := &nestedWaitingCancellationModel{}
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})

	leaf := agent.New(agent.Config{
		Name: "waiting-cancellation-leaf",
		Actions: []agent.Action{agent.NewAction(
			"wait",
			func(ctx context.Context, _ *core.ProcessContext, input nestedRestoreInput) (nestedRestoreOutput, error) {
				approved, err := hitl.Interrupt[bool](ctx, "approval-leaf", "approve leaf?")
				if err != nil || !approved {
					return nestedRestoreOutput{}, err
				}
				return nestedRestoreOutput{Value: input.Value * 2}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[nestedRestoreOutput](core.GoalConfig{Description: "leaf output"}),
		},
	})
	leafDeployment, err := engine.Deploy(t.Context(), leaf)
	if err != nil {
		t.Fatal(err)
	}
	leafTool, err := runtime.NewAgentTool[nestedRestoreInput, nestedRestoreOutput](engine, leafDeployment)
	if err != nil {
		t.Fatal(err)
	}
	middle := agent.New(agent.Config{
		Name: "waiting-cancellation-middle",
		Actions: []agent.Action{agent.NewAction(
			"delegate-to-leaf",
			func(ctx context.Context, _ *core.ProcessContext, input nestedRestoreInput) (nestedRestoreOutput, error) {
				arguments, err := json.Marshal(input)
				if err != nil {
					return nestedRestoreOutput{}, err
				}
				output, err := leafTool.Call(ctx, string(arguments))
				if err != nil {
					return nestedRestoreOutput{}, err
				}
				var result nestedRestoreOutput
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					return nestedRestoreOutput{}, err
				}
				return result, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[nestedRestoreOutput](core.GoalConfig{Description: "middle output"}),
		},
	})
	middleDeployment, err := engine.Deploy(t.Context(), middle)
	if err != nil {
		t.Fatal(err)
	}
	middleTool, err := runtime.NewAgentTool[nestedRestoreInput, nestedRestoreOutput](engine, middleDeployment)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tools.NewRegistry(middleTool)
	if err != nil {
		t.Fatal(err)
	}
	rootDefinition := managedInteractionAgent(
		t,
		"waiting-cancellation-managed-root",
		registry,
		interaction.Limits{},
	)
	mustDeploy(t, engine, rootDefinition)

	runHandle, err := engine.Start(t.Context(), rootDefinition, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitRun(t, runHandle); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	root := runHandle.Process()
	pending, err := engine.PendingSuspensions(t.Context(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].SuspensionID != "approval-leaf" {
		t.Fatalf("pending suspensions = %#v, want leaf approval", pending)
	}
	leafProcessID := pending[0].ProcessID

	plan, err := engine.PlanWaitingSubtreeCancellation(
		t.Context(),
		root.ID(),
		leafProcessID,
	)
	if err != nil {
		t.Fatalf("PlanWaitingSubtreeCancellation: %v", err)
	}
	if pending := plan.PendingSuspensions(); len(pending) != 0 {
		t.Fatalf("planned pending suspensions = %#v, want none", pending)
	}
	if err := engine.ApplyWaitingSubtreeCancellation(t.Context(), plan); err != nil {
		t.Fatalf("ApplyWaitingSubtreeCancellation: %v", err)
	}
	if err := engine.Continue(t.Context(), root.ID()); err != nil {
		t.Fatalf("Continue framework-ready ancestor chain: %v", err)
	}
	if root.Status() != core.StatusCompleted {
		t.Fatalf("root status = %s, failure=%v; want completed", root.Status(), root.Failure())
	}
	if model.Calls() != 2 {
		t.Fatalf("model calls = %d, want initial + error continuation", model.Calls())
	}
}

type waitingCancellationModel struct {
	mu    sync.Mutex
	calls int
}

func (m *waitingCancellationModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{
				ID:        "call-first",
				Name:      "waiting-cancellation-first",
				Arguments: `{"Value":21}`,
			}),
			chat.NewToolCallPart(chat.ToolCall{
				ID:        "call-second",
				Name:      "waiting-cancellation-second",
				Arguments: `{"Value":21}`,
			}),
		)
		return chat.NewResponse(chat.Choice{
			Index:        0,
			Message:      &message,
			FinishReason: chat.FinishReasonToolCalls,
		})
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != chat.RoleTool || len(last.Parts) != 2 {
		return nil, errors.New("managed cancellation continuation has invalid tool message")
	}
	var canceled, completed int
	for _, part := range last.Parts {
		result := part.ToolResult
		switch {
		case result == nil:
			return nil, errors.New("managed cancellation continuation has an empty tool result")
		case result.IsError && result.Result == "error: delegated child canceled":
			canceled++
		case !result.IsError && result.Result == `{"Value":42}`:
			completed++
		}
	}
	if canceled != 1 || completed != 1 {
		return nil, errors.New("managed cancellation continuation did not preserve canceled and completed results")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("complete"))
	return chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
}

func (m *waitingCancellationModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type nestedWaitingCancellationModel struct {
	mu    sync.Mutex
	calls int
}

func (m *nestedWaitingCancellationModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID:        "call-middle",
			Name:      "waiting-cancellation-middle",
			Arguments: `{"Value":21}`,
		}))
		return chat.NewResponse(chat.Choice{
			Index:        0,
			Message:      &message,
			FinishReason: chat.FinishReasonToolCalls,
		})
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != chat.RoleTool || len(last.Parts) != 1 ||
		last.Parts[0].ToolResult == nil ||
		!last.Parts[0].ToolResult.IsError ||
		!strings.Contains(last.Parts[0].ToolResult.Result, runtime.ErrChildProcessCanceled.Error()) {
		return nil, errors.New("managed ancestor did not receive the canceled descendant failure")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("recovered"))
	return chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
}

func (m *nestedWaitingCancellationModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func managedChildRegistry(t *testing.T, engine *runtime.Engine) *tools.Registry {
	t.Helper()
	var childTools []tool.Tool
	for _, definition := range []struct {
		name    string
		pauseID string
	}{
		{name: "waiting-cancellation-first", pauseID: "approval-first"},
		{name: "waiting-cancellation-second", pauseID: "approval-second"},
	} {
		child := agent.New(agent.Config{
			Name: definition.name,
			Actions: []agent.Action{agent.NewAction(
				"approve-and-double",
				func(ctx context.Context, _ *core.ProcessContext, input nestedRestoreInput) (nestedRestoreOutput, error) {
					approved, err := hitl.Interrupt[bool](ctx, definition.pauseID, "approve child?")
					if err != nil || !approved {
						return nestedRestoreOutput{}, err
					}
					return nestedRestoreOutput{Value: input.Value * 2}, nil
				},
				core.ActionConfig{},
			)},
			Goals: []*agent.Goal{
				agent.NewOutputGoal[nestedRestoreOutput](core.GoalConfig{Description: "child output"}),
			},
		})
		deployment, err := engine.Deploy(t.Context(), child)
		if err != nil {
			t.Fatal(err)
		}
		childTool, err := runtime.NewAgentTool[nestedRestoreInput, nestedRestoreOutput](engine, deployment)
		if err != nil {
			t.Fatal(err)
		}
		childTools = append(childTools, childTool)
	}
	registry, err := tools.NewRegistry(childTools...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func startWaitingNestedRestoreTree(
	t *testing.T,
	engine *runtime.Engine,
	parent *core.Agent,
) *runtime.Process {
	t.Helper()
	runHandle, err := engine.Start(
		t.Context(),
		parent,
		core.Input(nestedRestoreInput{Value: 21}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitRun(t, runHandle); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	root := runHandle.Process()
	if root.Status() != core.StatusWaiting {
		t.Fatalf("root status = %s, want waiting", root.Status())
	}
	return root
}

func onlyChildSnapshot(
	t *testing.T,
	tree core.ProcessSnapshotTree,
	parentID string,
) core.ProcessSnapshot {
	t.Helper()
	var children []core.ProcessSnapshot
	for _, snapshot := range tree.Snapshots {
		if snapshot.ParentID == parentID {
			children = append(children, snapshot)
		}
	}
	if len(children) != 1 {
		t.Fatalf("children of %q = %#v, want exactly one", parentID, children)
	}
	return children[0]
}
