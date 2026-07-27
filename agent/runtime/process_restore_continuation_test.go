package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
)

type restoreGateInput struct{}

type restoreGateOutput struct {
	Approved bool
}

func TestRestoreAnsweredSuspensionContinues(t *testing.T) {
	source := agent.MustNewEngine(runtime.Config{})
	definition := restoreGateAgent("restore-answered")
	mustDeploy(t, source, definition)

	segment, err := source.Start(t.Context(), definition, core.Input(restoreGateInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	process := segment.Process()
	if err := source.Resume(t.Context(), process.ID(), "restore-approval", true); err != nil {
		t.Fatal(err)
	}

	tree, err := source.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	root := snapshotByID(t, tree, process.ID())
	if root.Suspension == nil || !root.Suspension.Responded() {
		t.Fatalf("answered snapshot suspension = %#v", root.Suspension)
	}
	if err := runtime.ValidateResumableSnapshot(root); err != nil {
		t.Fatalf("ValidateResumableSnapshot(answered): %v", err)
	}

	target := agent.MustNewEngine(runtime.Config{})
	equivalent := restoreGateAgent("restore-answered")
	mustDeploy(t, target, equivalent)
	restored, err := target.RestoreTree(t.Context(), tree, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreTree(answered): %v", err)
	}
	if err := target.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("Continue(restored answered): %v", err)
	}
	result, ok := core.Result[restoreGateOutput](restored)
	if !ok || !result.Approved {
		t.Fatalf("restored result = %#v, %v", result, ok)
	}
}

func TestProducerPayloadNeverBecomesFrameworkState(t *testing.T) {
	source := agent.MustNewEngine(runtime.Config{})
	definition := producerToolSuspensionAgent()
	mustDeploy(t, source, definition)

	segment, err := source.Start(t.Context(), definition, core.Input(restoreGateInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	process := segment.Process()
	if process.Status() != core.StatusWaiting {
		t.Fatalf("process status = %s, want waiting", process.Status())
	}

	tree, err := source.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	root := snapshotByID(t, tree, process.ID())
	if root.Suspension == nil {
		t.Fatal("snapshot has no suspension")
	}
	if root.Suspension.FrameworkState != nil {
		t.Fatalf("producer suspension acquired framework state %s", root.Suspension.FrameworkState)
	}
	const producerPayload = `{"schema_version":2,"kind":"managed_interaction","owner":"producer"}`
	if string(root.Suspension.Payload) != producerPayload {
		t.Fatalf("producer payload = %s, want %s", root.Suspension.Payload, producerPayload)
	}
	if err := runtime.ValidateResumableSnapshot(root); err != nil {
		t.Fatalf("ValidateResumableSnapshot(producer tool suspension): %v", err)
	}

	target := agent.MustNewEngine(runtime.Config{})
	equivalent := producerToolSuspensionAgent()
	mustDeploy(t, target, equivalent)
	restored, err := target.RestoreTree(t.Context(), tree, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreTree(producer tool suspension): %v", err)
	}
	if err := target.Resume(t.Context(), restored.ID(), "producer-tool", true); err != nil {
		t.Fatal(err)
	}
	if err := target.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatal(err)
	}
	result, ok := core.Result[restoreGateOutput](restored)
	if !ok || !result.Approved {
		t.Fatalf("restored producer result = %#v, %v", result, ok)
	}
}

type nestedRestoreInput struct {
	Value int
}

type nestedRestoreOutput struct {
	Value int
}

func TestRestoreAnsweredNestedSuspensionContinuesWholeBranch(t *testing.T) {
	source := agent.MustNewEngine(runtime.Config{})
	sourceParent, sourceRef := deployNestedRestoreAgents(t, source)

	segment, err := source.Start(
		t.Context(),
		sourceParent,
		core.Input(nestedRestoreInput{Value: 21}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	root := segment.Process()
	if err := source.Resume(t.Context(), root.ID(), "nested-approval", true); err != nil {
		t.Fatal(err)
	}
	tree, err := source.SnapshotTree(t.Context(), root.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Snapshots) != 2 {
		t.Fatalf("answered nested snapshot count = %d, want 2", len(tree.Snapshots))
	}
	for _, snapshot := range tree.Snapshots {
		if snapshot.Status != core.StatusWaiting || snapshot.Suspension == nil || !snapshot.Suspension.Responded() {
			t.Fatalf("answered nested snapshot = %#v", snapshot)
		}
	}

	target := agent.MustNewEngine(runtime.Config{})
	_, targetRef := deployNestedRestoreAgents(t, target)
	if targetRef != sourceRef {
		t.Fatalf("equivalent parent deployment = %s, want %s", targetRef, sourceRef)
	}
	restored, err := target.RestoreTree(t.Context(), tree, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RestoreTree(answered nested): %v", err)
	}
	if err := target.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("Continue(answered nested): %v", err)
	}
	result, ok := core.Result[nestedRestoreOutput](restored)
	if !ok || result.Value != 42 {
		t.Fatalf("restored nested result = %#v, %v", result, ok)
	}
}

func restoreGateAgent(name string) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: name,
		Actions: []agent.Action{agent.NewAction(
			"gate",
			func(ctx context.Context, _ *core.ProcessContext, _ restoreGateInput) (restoreGateOutput, error) {
				approved, err := hitl.Interrupt[bool](ctx, "restore-approval", "approve?")
				return restoreGateOutput{Approved: approved}, err
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[restoreGateOutput](core.GoalConfig{Description: "approved"}),
		},
	})
}

func producerToolSuspensionAgent() *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: "producer-tool-suspension",
		Actions: []agent.Action{agent.NewAction(
			"wait-for-tool",
			func(ctx context.Context, _ *core.ProcessContext, _ restoreGateInput) (restoreGateOutput, error) {
				if current := core.ProcessViewFrom(ctx).Suspension(); current != nil && current.Responded() {
					var approved bool
					if err := json.Unmarshal(current.Response, &approved); err != nil {
						return restoreGateOutput{}, err
					}
					return restoreGateOutput{Approved: approved}, nil
				}
				return restoreGateOutput{}, &interaction.SuspendedError{Suspension: interaction.Suspension{
					SchemaVersion: interaction.SuspensionSchemaVersion,
					ID:            "producer-tool",
					Kind:          interaction.SuspensionTool,
					Prompt:        json.RawMessage(`"continue?"`),
					ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
					Payload:       json.RawMessage(`{"schema_version":2,"kind":"managed_interaction","owner":"producer"}`),
					CreatedAt:     time.Now(),
				}}
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[restoreGateOutput](core.GoalConfig{Description: "producer resumed"}),
		},
	})
}

func deployNestedRestoreAgents(t *testing.T, engine *runtime.Engine) (*core.Agent, core.DeploymentRef) {
	t.Helper()
	child := agent.New(agent.AgentConfig{
		Name: "nested-restore-child",
		Actions: []agent.Action{agent.NewAction(
			"approve-and-double",
			func(ctx context.Context, _ *core.ProcessContext, input nestedRestoreInput) (nestedRestoreOutput, error) {
				approved, err := hitl.Interrupt[bool](ctx, "nested-approval", "approve child?")
				if err != nil {
					return nestedRestoreOutput{}, err
				}
				if !approved {
					return nestedRestoreOutput{}, nil
				}
				return nestedRestoreOutput{Value: input.Value * 2}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[nestedRestoreOutput](core.GoalConfig{Description: "child output"}),
		},
	})
	childDeployment, err := engine.Deploy(t.Context(), child)
	if err != nil {
		t.Fatal(err)
	}
	childTool, err := runtime.NewAgentTool[nestedRestoreInput, nestedRestoreOutput](engine, childDeployment)
	if err != nil {
		t.Fatal(err)
	}
	parent := agent.New(agent.AgentConfig{
		Name: "nested-restore-parent",
		Actions: []agent.Action{agent.NewAction(
			"delegate",
			func(ctx context.Context, _ *core.ProcessContext, input nestedRestoreInput) (nestedRestoreOutput, error) {
				arguments, err := json.Marshal(input)
				if err != nil {
					return nestedRestoreOutput{}, err
				}
				output, err := childTool.Call(ctx, string(arguments))
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
			agent.NewOutputGoal[nestedRestoreOutput](core.GoalConfig{Description: "parent output"}),
		},
	})
	parentDeployment, err := engine.Deploy(t.Context(), parent)
	if err != nil {
		t.Fatal(err)
	}
	return parent, parentDeployment.Ref()
}

func snapshotByID(t *testing.T, tree core.ProcessSnapshotTree, id string) core.ProcessSnapshot {
	t.Helper()
	for _, snapshot := range tree.Snapshots {
		if snapshot.ID == id {
			return snapshot
		}
	}
	t.Fatalf("snapshot %q is missing", id)
	return core.ProcessSnapshot{}
}
