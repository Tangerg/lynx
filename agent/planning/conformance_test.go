package planning_test

import (
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
	"github.com/Tangerg/scope/agent/planning"
)

func TestDefinitionConformance(t *testing.T) {
	condition := mustCondition(t, "conformance.ready", planning.True)
	definition := newManagedDefinition(t, managedDeploymentConfig{
		name: "planning.conformance",
		goal: mustGoal(t, condition),
	})
	input, err := agent.EncodeInput(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	agenttest.RunDefinitionConformance(t, agenttest.DefinitionConformanceConfig{
		Definition: definition,
		Input:      input,
	})
}
