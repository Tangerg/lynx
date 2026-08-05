package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type typedCount struct {
	Count int `json:"count"`
}

type typedFixtureDefinition struct {
	descriptor Descriptor
	starts     int
}

func (definition *typedFixtureDefinition) Descriptor() Descriptor { return definition.descriptor }

func (definition *typedFixtureDefinition) Start(input Input) (Execution, error) {
	definition.starts++
	return &typedFixtureExecution{state: input.JSON()}, nil
}

func (definition *typedFixtureDefinition) Restore(state ExecutionState) (Execution, error) {
	return &typedFixtureExecution{state: state.Payload()}, nil
}

type typedFixtureExecution struct {
	state json.RawMessage
}

func (execution *typedFixtureExecution) Step(context.Context, []Signal) (Transition, error) {
	output, err := ParseOutput(execution.state)
	if err != nil {
		return Transition{}, err
	}
	return Complete(0, output)
}

func (execution *typedFixtureExecution) Snapshot() (ExecutionState, error) {
	return NewExecutionState("fixture", 1, execution.state)
}

func TestTypedAdapterKeepsDefinitionErased(t *testing.T) {
	messageDefinition := newTypedFixtureDefinition[wireFixture](t, "fixture.message")
	countDefinition := newTypedFixtureDefinition[typedCount](t, "fixture.count")

	message, err := NewTyped[wireFixture, wireFixture](messageDefinition)
	if err != nil {
		t.Fatal(err)
	}
	count, err := NewTyped[typedCount, typedCount](countDefinition)
	if err != nil {
		t.Fatal(err)
	}
	erased := []Definition{message.Definition(), count.Definition()}
	if len(erased) != 2 || erased[0].Descriptor().Name() != "fixture.message" || erased[1].Descriptor().Name() != "fixture.count" {
		t.Fatalf("heterogeneous erased Definitions = %+v", erased)
	}

	input, err := message.EncodeInput(wireFixture{Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := message.Definition().Start(input)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := execution.Step(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	output, ok := transition.Output()
	if !ok {
		t.Fatal("fixture did not complete")
	}
	decoded, err := message.DecodeOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Message != "hello" {
		t.Fatalf("decoded output = %+v", decoded)
	}
}

func TestTypedAdapterRejectsSchemaMismatchAtEdge(t *testing.T) {
	definition := newTypedFixtureDefinition[wireFixture](t, "fixture.message")
	adapter, err := NewTyped[typedCount, wireFixture](definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.EncodeInput(typedCount{Count: 3}); !errors.Is(err, ErrInvalidTypedAdapter) {
		t.Fatalf("EncodeInput schema mismatch error = %v, want ErrInvalidTypedAdapter", err)
	}
	if definition.starts != 0 {
		t.Fatalf("Definition.Start called %d times after edge validation failed", definition.starts)
	}
}

func TestTypedAdapterRejectsTypedNilDefinition(t *testing.T) {
	var definition *typedFixtureDefinition
	if _, err := NewTyped[wireFixture, wireFixture](definition); !errors.Is(err, ErrInvalidTypedAdapter) {
		t.Fatalf("NewTyped(typed nil) error = %v, want ErrInvalidTypedAdapter", err)
	}
}

func newTypedFixtureDefinition[T any](t *testing.T, name string) *typedFixtureDefinition {
	t.Helper()
	schema, err := SchemaFor[T]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name:         name,
		Description:  "A typed adapter contract test fixture.",
		Version:      "1.0.0",
		InputSchema:  schema,
		OutputSchema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &typedFixtureDefinition{descriptor: descriptor}
}
