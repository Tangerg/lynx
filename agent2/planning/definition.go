package planning

import (
	"encoding/json"
	"fmt"
	"slices"

	agent "github.com/Tangerg/lynx/agent2"
)

const (
	executionStateKind          = "planning"
	executionStateSchemaVersion = 3
)

// DefinitionConfig contains one immutable managed Planning behavior. Goal,
// Planner, and Action bindings are fixed for the exact Deployment; only Input
// varies per Process.
type DefinitionConfig struct {
	// Name is the stable qualified Definition name.
	Name string

	// Description states the managed goal-directed behavior for discovery.
	Description string

	// Version is the semantic version of the Definition contract.
	Version string

	// InputSchema is the authoritative schema for opaque task input passed to
	// Observer, ActionExecutor, and child input functions.
	InputSchema agent.Schema

	// Goal is the immutable target state.
	Goal Goal

	// Actions binds every predictive Action to exactly one execution mechanism.
	Actions []ActionBinding

	// Planner selects Actions from each newly observed WorldState.
	Planner Planner

	// MaxActionAttempts bounds external Action attempts. It must be positive.
	MaxActionAttempts uint32
}

// Definition is an immutable Planning Strategy definition. It contains no
// Observer or ActionExecutor; those I/O capabilities belong to its
// Deployment-bound Dispatcher.
type Definition struct {
	descriptor        agent.Descriptor
	goal              Goal
	bindings          []ActionBinding
	byName            map[string]ActionBinding
	planner           Planner
	maxActionAttempts uint32
}

// NewDefinition validates config and constructs a managed Planning Definition.
func NewDefinition(config DefinitionConfig) (*Definition, error) {
	if !config.InputSchema.Valid() || !config.Goal.Valid() || isNilImplementation(config.Planner) || config.MaxActionAttempts == 0 {
		return nil, ErrInvalidDefinitionConfig
	}
	bindings := slices.Clone(config.Actions)
	byName := make(map[string]ActionBinding, len(bindings))
	for index, binding := range bindings {
		if !binding.Valid() {
			return nil, fmt.Errorf("%w: Actions[%d]", ErrInvalidDefinitionConfig, index)
		}
		name := binding.action.name
		if _, duplicate := byName[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Action %q", ErrInvalidDefinitionConfig, name)
		}
		byName[name] = binding
	}
	outputSchema, err := agent.ParseSchema(json.RawMessage(planningOutputSchema))
	if err != nil {
		return nil, fmt.Errorf("%w: output schema: %w", ErrInvalidDefinitionConfig, err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: config.Name, Description: config.Description, Version: config.Version,
		InputSchema: config.InputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: descriptor: %w", ErrInvalidDefinitionConfig, err)
	}
	return &Definition{
		descriptor: descriptor, goal: config.Goal, bindings: bindings, byName: byName,
		planner: config.Planner, maxActionAttempts: config.MaxActionAttempts,
	}, nil
}

// Descriptor returns the immutable managed Planning contract.
func (definition *Definition) Descriptor() agent.Descriptor {
	if definition == nil {
		return agent.Descriptor{}
	}
	return definition.descriptor
}

// Start creates a fresh Planning Execution from validated opaque task input.
func (definition *Definition) Start(input agent.Input) (agent.Execution, error) {
	if !definition.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	if err := definition.descriptor.ValidateInput(input); err != nil {
		return nil, err
	}
	state := executionState{Phase: phaseReadyObservation, Input: input.JSON()}
	return &execution{definition: definition, state: state}, nil
}

// Restore recreates a Planning Execution solely from its opaque, versioned
// state and this exact Definition.
func (definition *Definition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if !definition.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	if state.Kind() != executionStateKind || state.SchemaVersion() != executionStateSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported kind or schema version", ErrInvalidExecutionState)
	}
	var decoded executionState
	if err := decodeStrict(state.Payload(), &decoded); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidExecutionState, err)
	}
	if err := decoded.validate(definition); err != nil {
		return nil, err
	}
	return &execution{definition: definition, state: decoded}, nil
}

func (definition *Definition) valid() bool {
	if definition == nil || !definition.descriptor.Valid() || !definition.goal.Valid() ||
		isNilImplementation(definition.planner) || definition.maxActionAttempts == 0 || len(definition.bindings) != len(definition.byName) {
		return false
	}
	for _, binding := range definition.bindings {
		if !binding.Valid() || definition.byName[binding.action.name].action.name != binding.action.name {
			return false
		}
	}
	return true
}

func (definition *Definition) binding(name string) (ActionBinding, bool) {
	binding, found := definition.byName[name]
	return binding, found
}

func (definition *Definition) problem(state WorldState, excluded []string) (Problem, error) {
	actions := make([]Action, 0, len(definition.bindings))
	for _, binding := range definition.bindings {
		if !slices.Contains(excluded, binding.action.name) {
			actions = append(actions, binding.action)
		}
	}
	return NewProblem(state, definition.goal, actions...)
}

func encodeExecutionState(state executionState) (agent.ExecutionState, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return agent.ExecutionState{}, fmt.Errorf("planning: encode execution state: %w", err)
	}
	return agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
}

const planningOutputSchema = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "outcome":{"enum":["achieved","unreachable","stuck"]},
    "world_state":{
      "type":"object",
      "additionalProperties":false,
      "properties":{
        "conditions":{
          "type":"array",
          "items":{
            "type":"object",
            "additionalProperties":false,
            "properties":{
              "key":{"type":"string","pattern":"^[a-z][a-z0-9._-]{0,127}$"},
              "truth":{"enum":["false","true"]}
            },
            "required":["key","truth"]
          }
        }
      },
      "required":["conditions"]
    },
    "attempts":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
		  "action_name":{"type":"string","pattern":"^[a-z][a-z0-9._-]{0,127}$"},
          "status":{"enum":["succeeded","failed","unconfirmed"]},
          "diagnostic":{"type":"string","minLength":1,"maxLength":4096}
        },
		"required":["action_name","status"]
      }
    },
    "planning_passes":{"type":"integer","minimum":0,"maximum":4294967295}
  },
  "required":["outcome","world_state","attempts","planning_passes"]
}`
