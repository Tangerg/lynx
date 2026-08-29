package workflow_test

import (
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
)

func TestDefinitionConformance(t *testing.T) {
	transform := mustTransform(t, "increment", func(input numberInput) (numberOutput, error) {
		return numberOutput{Value: input.Value + 1}, nil
	})
	definition := mustDefinition(t, "workflow.conformance", transform)
	input, err := agent.EncodeInput(numberInput{Value: 7})
	if err != nil {
		t.Fatal(err)
	}
	agenttest.RunDefinitionConformance(t, agenttest.DefinitionConformanceConfig{
		Definition: definition,
		Input:      input,
	})
}
