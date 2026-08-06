package workflow_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/workflow"
)

func TestMapUsesManagedChildrenAndPreservesItemOrder(t *testing.T) {
	child := mustDeployment(t, mustDefinition(t, "test.workflow.map_child",
		mustTransform(t, "double", func(input forkInput) (numberOutput, error) {
			return numberOutput{Value: input.Value * 2}, nil
		}),
	), "map-child")
	stage, err := workflow.Map(workflow.MapConfig[forkInput, numberOutput]{
		ID: "items", Deployment: child, Budget: mustBudget(t),
		Concurrency: 2, ItemLimit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stage.Valid() || stage.Kind() != workflow.StageKindMap || stage.Kind().String() != "map" {
		t.Fatalf("Map Stage = %#v", stage)
	}
	deployment := mustDeployment(t, mustDefinition(t, "test.workflow.map", stage), "map")
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver: deploymentResolver{child.Reference(): child},
	})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := agent.EncodeInput([]forkInput{{Value: 3}, {Value: 1}, {Value: 2}})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	output := decodeCompleted[[]numberOutput](t, result)
	if !slices.Equal(output, []numberOutput{{Value: 6}, {Value: 2}, {Value: 4}}) {
		t.Fatalf("Map output = %#v", output)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMapEmptyInputProducesNonNilEmptyOutput(t *testing.T) {
	child := mustDeployment(t, mustDefinition(t, "test.workflow.empty_map_child",
		mustTransform(t, "identity", func(input forkInput) (numberOutput, error) {
			return numberOutput(input), nil
		}),
	), "empty-map-child")
	stage, err := workflow.Map(workflow.MapConfig[forkInput, numberOutput]{
		ID: "items", Deployment: child, Budget: mustBudget(t),
		Concurrency: 1, ItemLimit: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment := mustDeployment(t, mustDefinition(t, "test.workflow.empty_map", stage), "empty-map")
	engine, _ := agent.NewEngine(agent.EngineConfig{})
	input, _ := agent.EncodeInput([]forkInput{})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	output := decodeCompleted[[]numberOutput](t, result)
	if output == nil || len(output) != 0 {
		t.Fatalf("empty Map output = %#v", output)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMapRejectsInputAboveItemLimitBeforeStartingChildren(t *testing.T) {
	child := mustDeployment(t, mustDefinition(t, "test.workflow.limited_map_child",
		mustTransform(t, "identity", func(input forkInput) (numberOutput, error) {
			return numberOutput(input), nil
		}),
	), "limited-map-child")
	stage, err := workflow.Map(workflow.MapConfig[forkInput, numberOutput]{
		ID: "items", Deployment: child, Budget: mustBudget(t),
		Concurrency: 2, ItemLimit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment := mustDeployment(t, mustDefinition(t, "test.workflow.limited_map", stage), "limited-map")
	engine, _ := agent.NewEngine(agent.EngineConfig{})
	input, _ := agent.EncodeInput([]forkInput{{Value: 1}, {Value: 2}, {Value: 3}})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	failure, present := result.Termination().Failure()
	if result.Status() != agent.StatusFailed || !present || failure.Code() != "workflow.map.item_limit_exceeded" {
		t.Fatalf("Map termination = %#v", result.Termination())
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMapRequiresExplicitLimitsAndMatchingChildContract(t *testing.T) {
	child := mustDeployment(t, mustDefinition(t, "test.workflow.valid_map_child",
		mustTransform(t, "identity", func(input forkInput) (numberOutput, error) {
			return numberOutput(input), nil
		}),
	), "valid-map-child")
	wrong := mustDeployment(t, mustDefinition(t, "test.workflow.wrong_map_child",
		mustTransform(t, "wrong", func(textValue) (numberOutput, error) {
			return numberOutput{}, nil
		}),
	), "wrong-map-child")
	valid := workflow.MapConfig[forkInput, numberOutput]{
		ID: "items", Deployment: child, Budget: mustBudget(t), Concurrency: 1, ItemLimit: 2,
	}
	for name, config := range map[string]workflow.MapConfig[forkInput, numberOutput]{
		"zero concurrency": {ID: valid.ID, Deployment: child, Budget: valid.Budget, ItemLimit: 2},
		"zero item limit":  {ID: valid.ID, Deployment: child, Budget: valid.Budget, Concurrency: 1},
		"oversized concurrency": {
			ID: valid.ID, Deployment: child, Budget: valid.Budget, Concurrency: 3, ItemLimit: 2,
		},
		"schema mismatch": {
			ID: valid.ID, Deployment: wrong, Budget: valid.Budget, Concurrency: 1, ItemLimit: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := workflow.Map(config); !errors.Is(err, workflow.ErrInvalidStage) {
				t.Fatalf("Map error = %v", err)
			}
		})
	}
}
