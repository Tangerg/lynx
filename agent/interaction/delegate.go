package interaction

import (
	"fmt"
	"strings"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
)

const maxDelegateDescriptionBytes = 4096

// DelegateConfig exposes one exact child Deployment as a model-selectable
// Interaction capability. Name and Description are written for the model;
// lifecycle identity and resource authority remain frozen Framework values.
type DelegateConfig struct {
	// Name is the provider-compatible model Tool name.
	Name string

	// Description tells the model when and why to delegate this work.
	Description string

	// Deployment is the exact child behavior binding. The Delegate retains only
	// its immutable reference and Descriptor schemas.
	Deployment agent.Deployment

	// Budget is permanently allocated from the parent for each invocation.
	Budget agent.Budget

	// Capabilities is the attenuated authority set granted to each child.
	Capabilities agent.CapabilitySet
}

// Delegate is an immutable, model-visible binding to one exact managed child
// Deployment. It is a composition value owned by Interaction, not an
// executable Tool or a second Process-start entry point.
type Delegate struct {
	definition    chat.ToolDefinition
	deploymentRef agent.DeploymentRef
	inputSchema   agent.Schema
	outputSchema  agent.Schema
	budget        agent.Budget
	capabilities  agent.CapabilitySet
}

// NewDelegate validates and freezes one managed worker binding. The target
// input schema must be a JSON object because models invoke Delegates through
// the same ToolCall protocol as ordinary Tools.
func NewDelegate(config DelegateConfig) (Delegate, error) {
	if !config.Deployment.Valid() || !config.Budget.Valid() || !config.Capabilities.Valid() ||
		config.Description == "" || strings.TrimSpace(config.Description) != config.Description ||
		len(config.Description) > maxDelegateDescriptionBytes {
		return Delegate{}, ErrInvalidDelegate
	}
	descriptor := config.Deployment.Descriptor()
	definition := chat.ToolDefinition{
		Name: config.Name, Description: config.Description,
		InputSchema: descriptor.InputSchema().JSON(),
	}
	if err := definition.Validate(); err != nil {
		return Delegate{}, fmt.Errorf("%w: model contract: %w", ErrInvalidDelegate, err)
	}
	return Delegate{
		definition: definition.Clone(), deploymentRef: config.Deployment.DeploymentRef(),
		inputSchema: descriptor.InputSchema(), outputSchema: descriptor.OutputSchema(), budget: config.Budget,
		capabilities: config.Capabilities,
	}, nil
}

// Valid reports whether the Delegate contains one exact, internally
// consistent model and child-execution contract.
func (d Delegate) Valid() bool {
	return d.definition.Validate() == nil && d.definition.Description != "" &&
		strings.TrimSpace(d.definition.Description) == d.definition.Description &&
		len(d.definition.Description) <= maxDelegateDescriptionBytes &&
		d.deploymentRef.Valid() && d.inputSchema.Valid() && d.outputSchema.Valid() &&
		d.budget.Valid() && d.capabilities.Valid()
}

func (d Delegate) clone() Delegate {
	d.definition = d.definition.Clone()
	return d
}

func (d Delegate) validateInput(input agent.Input) error {
	if !d.Valid() {
		return ErrInvalidDelegate
	}
	return d.inputSchema.ValidateInput(input)
}
