package interaction_test

import (
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
)

func TestDefinitionConformance(t *testing.T) {
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "interaction.conformance",
		Description:   "Verify the Interaction Definition and Execution contract.",
		MaxModelCalls: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodeInput(interaction.Input{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	agenttest.RunDefinitionConformance(t, agenttest.DefinitionConformanceConfig{
		Definition: definition,
		Input:      input,
	})
}
