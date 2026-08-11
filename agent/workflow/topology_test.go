package workflow_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/workflow"
)

func TestDefinitionTopologyProjectsEverySealedStageKind(t *testing.T) {
	numberChild := mustTopologyDeployment(
		t,
		"test.topology.number",
		func(input numberInput) (numberOutput, error) {
			return numberOutput(input), nil
		},
	)
	alternateNumberChild := mustTopologyDeployment(
		t,
		"test.topology.alternate_number",
		func(input numberInput) (numberOutput, error) {
			return numberOutput(input), nil
		},
	)
	identityChild := mustTopologyDeployment(
		t,
		"test.topology.identity",
		func(input numberInput) (numberInput, error) { return input, nil },
	)
	budget := mustBudget(t)
	capability, err := agent.ParseCapability("test.topology.execute")
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := agent.NewCapabilitySet(capability)
	if err != nil {
		t.Fatal(err)
	}

	transform := mustTransform(t, "transform", func(input numberInput) (numberOutput, error) {
		return numberOutput(input), nil
	})
	call, err := workflow.Call(workflow.CallConfig{
		ID: "call", Deployment: numberChild, Budget: budget, Capabilities: capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	switcher, err := workflow.Switch(workflow.SwitchConfig[numberInput]{
		ID:     "switch",
		Select: func(numberInput) (string, error) { return "primary", nil },
		Cases: []workflow.SwitchCase{
			{ID: "primary", Deployment: numberChild, Budget: budget, Capabilities: capabilities},
			{ID: "fallback", Deployment: alternateNumberChild, Budget: budget, Capabilities: capabilities},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := workflow.Fork(workflow.ForkConfig[numberInput, numberOutput, numberOutput]{
		ID: "fork",
		Branches: []workflow.ForkBranch{
			{ID: "left", Deployment: numberChild, Budget: budget, Capabilities: capabilities},
			{ID: "right", Deployment: alternateNumberChild, Budget: budget, Capabilities: capabilities},
		},
		WindowSize: 1,
		Reduce:     func(outputs []numberOutput) (numberOutput, error) { return outputs[0], nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	mapper, err := workflow.Map[numberInput, numberOutput](workflow.MapConfig[numberInput, numberOutput]{
		ID: "map", Deployment: numberChild, Budget: budget, Capabilities: capabilities,
		WindowSize: 2, ItemLimit: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	loop, err := workflow.Loop(workflow.LoopConfig[numberInput]{
		ID: "loop", Body: identityChild, Budget: budget, Capabilities: capabilities, MaxIterations: 3,
		Predicate: func(numberInput) (bool, error) { return true, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	type expectedBinding struct {
		role       workflow.BindingRole
		id         string
		deployment agent.Deployment
	}
	tests := []struct {
		name          string
		stage         workflow.Stage
		kind          workflow.StageKind
		bindings      []expectedBinding
		windowSize    uint32
		itemLimit     uint32
		maxIterations uint32
	}{
		{name: "transform", stage: transform, kind: workflow.StageKindTransform},
		{name: "call", stage: call, kind: workflow.StageKindCall, bindings: []expectedBinding{
			{role: workflow.BindingRoleCall, deployment: numberChild},
		}},
		{name: "switch", stage: switcher, kind: workflow.StageKindSwitch, bindings: []expectedBinding{
			{role: workflow.BindingRoleCase, id: "primary", deployment: numberChild},
			{role: workflow.BindingRoleCase, id: "fallback", deployment: alternateNumberChild},
		}},
		{name: "fork", stage: fork, kind: workflow.StageKindFork, bindings: []expectedBinding{
			{role: workflow.BindingRoleBranch, id: "left", deployment: numberChild},
			{role: workflow.BindingRoleBranch, id: "right", deployment: alternateNumberChild},
		}, windowSize: 1},
		{name: "map", stage: mapper, kind: workflow.StageKindMap, bindings: []expectedBinding{
			{role: workflow.BindingRoleItem, deployment: numberChild},
		}, windowSize: 2, itemLimit: 4},
		{name: "loop", stage: loop, kind: workflow.StageKindLoop, bindings: []expectedBinding{
			{role: workflow.BindingRoleBody, deployment: identityChild},
		}, maxIterations: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := mustDefinition(t, "test.topology."+test.name, test.stage)
			topology := definition.Topology()
			if topology.Descriptor.Digest() != definition.Descriptor().Digest() || len(topology.Stages) != 1 {
				t.Fatalf("topology=%+v", topology)
			}
			stage := topology.Stages[0]
			if stage.Kind != test.kind || stage.WindowSize != test.windowSize ||
				stage.ItemLimit != test.itemLimit || stage.MaxIterations != test.maxIterations {
				t.Fatalf("stage=%+v", stage)
			}
			if stage.ID != test.name || !stage.InputSchema.Valid() ||
				!stage.OutputSchema.Valid() || len(stage.Bindings) != len(test.bindings) {
				t.Fatalf("stage contracts=%+v", stage)
			}
			for index, want := range test.bindings {
				binding := stage.Bindings[index]
				descriptor := want.deployment.Descriptor()
				if binding.Role != want.role || binding.ID != want.id ||
					binding.DeploymentRef != want.deployment.DeploymentRef() ||
					!bytes.Equal(binding.InputSchema.JSON(), descriptor.InputSchema().JSON()) ||
					!bytes.Equal(binding.OutputSchema.JSON(), descriptor.OutputSchema().JSON()) ||
					binding.Budget != budget ||
					!slices.Equal(binding.Capabilities.Values(), capabilities.Values()) {
					t.Fatalf("binding[%d]=%+v", index, binding)
				}
			}
			if _, err := json.Marshal(topology); err != nil {
				t.Fatalf("marshal Topology: %v", err)
			}
		})
	}
}

func TestDefinitionTopologyOwnsProjectionSlices(t *testing.T) {
	child := mustTopologyDeployment(
		t,
		"test.topology.ownership_child",
		func(input numberInput) (numberOutput, error) { return numberOutput(input), nil },
	)
	stage, err := workflow.Switch(workflow.SwitchConfig[numberInput]{
		ID:     "route",
		Select: func(numberInput) (string, error) { return "first", nil },
		Cases: []workflow.SwitchCase{
			{ID: "first", Deployment: child, Budget: mustBudget(t)},
			{ID: "second", Deployment: child, Budget: mustBudget(t)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := mustDefinition(t, "test.topology.ownership", stage)

	first := definition.Topology()
	first.Stages[0].ID = "changed"
	first.Stages[0].Bindings[0].ID = "changed"
	second := definition.Topology()
	if second.Stages[0].ID != "route" || second.Stages[0].Bindings[0].ID != "first" {
		t.Fatalf("later projection was mutated: %+v", second.Stages[0])
	}
}

func TestNilDefinitionTopologyIsZero(t *testing.T) {
	var definition *workflow.Definition
	if topology := definition.Topology(); topology.Descriptor.Valid() || len(topology.Stages) != 0 {
		t.Fatalf("nil Definition Topology=%+v", topology)
	}
}

func mustTopologyDeployment[I, O any](
	t *testing.T,
	name string,
	transform workflow.TransformFunc[I, O],
) agent.Deployment {
	t.Helper()
	stage, err := workflow.Transform("apply", transform)
	if err != nil {
		t.Fatal(err)
	}
	definition := mustDefinition(t, name, stage)
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: workflow.Dispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte(name + ":implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte(name + ":configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}
