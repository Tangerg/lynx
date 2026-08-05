package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
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

	runHandle, err := source.Start(t.Context(), definition, core.Input(restoreGateInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitRun(t, runHandle); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	process := runHandle.Process()
	if err := source.Respond(t.Context(), process.ID(), "restore-approval", true); err != nil {
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

func TestForeignSuspensionNeverAcquiresFrameworkState(t *testing.T) {
	source := agent.MustNewEngine(runtime.Config{})
	definition := producerToolSuspensionAgent()
	mustDeploy(t, source, definition)

	runHandle, err := source.Start(t.Context(), definition, core.Input(restoreGateInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitRun(t, runHandle); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	process := runHandle.Process()
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
	if err := target.Respond(t.Context(), restored.ID(), "producer-tool", true); err != nil {
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

	runHandle, err := source.Start(
		t.Context(),
		sourceParent,
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
	pending, err := source.PendingSuspensions(t.Context(), root.ID())
	if err != nil {
		t.Fatalf("PendingSuspensions: %v", err)
	}
	if len(pending) != 1 ||
		pending[0].ProcessID == root.ID() ||
		pending[0].SuspensionID != "nested-approval" {
		t.Fatalf("nested pending suspensions = %#v, want one child-owned approval", pending)
	}
	if err := source.Respond(t.Context(), root.ID(), "nested-approval", true); err != nil {
		t.Fatal(err)
	}
	pending, err = source.PendingSuspensions(t.Context(), root.ID())
	if err != nil {
		t.Fatalf("PendingSuspensions(answered): %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("answered pending suspensions = %#v, want none", pending)
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
	return agent.New(agent.Config{
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
	return agent.New(agent.Config{
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
					SchemaVersion:  interaction.SuspensionSchemaVersion,
					ID:             "producer-tool",
					Prompt:         json.RawMessage(`"continue?"`),
					ResponseSchema: json.RawMessage(`{"type":"boolean"}`),
					CreatedAt:      time.Now(),
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
	child := agent.New(agent.Config{
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
	parent := agent.New(agent.Config{
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

// TestUnrestorableSnapshotIsAlwaysClassifiable pins the contract a host recovery
// policy depends on: ValidateResumableSnapshot is a pure check over captured
// state, so every way it can fail has to report ErrInvalidSnapshot. Otherwise a
// host has to enumerate which failures mean "this capture is unusable", and a
// check added later silently falls outside that set — the parked run then
// surfaces as an internal error instead of being recovered as lost.
func TestUnrestorableSnapshotIsAlwaysClassifiable(t *testing.T) {
	source := agent.MustNewEngine(runtime.Config{})
	definition := restoreGateAgent("restore-classifiable")
	mustDeploy(t, source, definition)

	runHandle, err := source.Start(t.Context(), definition, core.Input(restoreGateInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitRun(t, runHandle); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	tree, err := source.SnapshotTree(t.Context(), runHandle.Process().ID())
	if err != nil {
		t.Fatal(err)
	}
	resumable := snapshotByID(t, tree, runHandle.Process().ID())
	if err := runtime.ValidateResumableSnapshot(resumable); err != nil {
		t.Fatalf("captured snapshot is not resumable: %v", err)
	}

	corrupt := map[string]func(*core.ProcessSnapshot){
		// A snapshot has to stay internally consistent to get past
		// ProcessSnapshot.Validate, so this drops the suspension along with the
		// status — otherwise the generic check fires first and the resumability
		// check is never reached.
		"terminal, with no continuation to resume": func(s *core.ProcessSnapshot) {
			s.Status = core.StatusCompleted
			s.Suspension = nil
		},
		"framework state is not JSON": func(s *core.ProcessSnapshot) {
			s.Suspension.FrameworkState = json.RawMessage(`{`)
		},
		"framework state has an unknown schema": func(s *core.ProcessSnapshot) {
			s.Suspension.FrameworkState = json.RawMessage(`{"schema_version":9999}`)
		},
		"framework state has an unknown field": func(s *core.ProcessSnapshot) {
			s.Suspension.FrameworkState = json.RawMessage(`{"schema_version":5,"invented":true}`)
		},
		"framework state has a trailing value": func(s *core.ProcessSnapshot) {
			s.Suspension.FrameworkState = json.RawMessage(`{"schema_version":5,"kind":"managed_interaction"} {}`)
		},
	}
	for name, corruption := range corrupt {
		t.Run(name, func(t *testing.T) {
			snapshot := resumable
			snapshot.Suspension = resumable.Suspension.Clone()
			corruption(&snapshot)

			err := runtime.ValidateResumableSnapshot(snapshot)
			if err == nil {
				t.Fatal("a snapshot that cannot be restored validated cleanly")
			}
			if !errors.Is(err, core.ErrInvalidSnapshot) {
				t.Fatalf("error = %v, want it to report ErrInvalidSnapshot", err)
			}
		})
	}
}
