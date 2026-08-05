package core

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// AgentConfig is the construction input for [NewAgent]. It is ordinary Go
// configuration: callers may assemble it freely, while Agent takes a
// defensive snapshot at construction.
type AgentConfig struct {
	// Name is the agent's identifier — required, unique within a
	// Engine.
	Name string

	// Description is the human-readable summary surfaced in tracing
	// and (when the agent is exposed externally) the LLM prompt.
	Description string

	// Version is an optional canonical MAJOR.MINOR.PATCH semantic version.
	// Empty means the definition is unversioned. Validation is owned by Agent so
	// callers do not need semver types in configuration.
	Version string

	// StuckPolicy is the recovery hook fired when the planner
	// returns no plan. Nil → transition to StatusStuck.
	StuckPolicy StuckPolicy

	// Actions are the planner-visible actions. ≥1 required.
	Actions []Action

	// Goals are the success criteria the planner picks among.
	Goals []*Goal

	// Conditions are user-supplied named predicates the world-state
	// determiner can evaluate alongside the auto-derived ones.
	Conditions []Condition

	// SnapshotState declares additional typed blackboard values owned by the
	// agent but not represented by action inputs or outputs. Generated workflows
	// use it for loop checkpoints and history. Every entry must be constructed
	// with NewBinding so snapshots can reconstruct its exact Go type.
	SnapshotState []Binding

	// PlannerName selects which planner the runtime uses for this
	// agent. It must match the [Extension.Name] of a planner
	// registered on the engine (or via process-scope
	// [ProcessOptions.Extensions]). Empty → framework default
	// ("goap"). Built-in names include goap, htn, reactive, utility,
	// and goal-first-utility.
	PlannerName string
}

// Agent is a read-only definition aggregate. Configuration remains transparent
// through AgentConfig, but a constructed Agent owns its identity, capability
// set, validation, and derived planning behavior. Runtime state belongs to
// Process and deployment identity belongs to runtime.Deployment.
type Agent struct {
	config AgentConfig
}

// AgentDescriptor is the immutable, non-executable projection of an agent
// definition. It exposes identity, planning declarations, and state schema
// while deliberately excluding actions, conditions, and recovery policies
// that can execute caller code.
type AgentDescriptor struct {
	name          string
	description   string
	version       string
	actions       []ActionDescriptor
	goals         []GoalDescriptor
	conditions    []ConditionDescriptor
	snapshotState []Binding
	plannerName   string
}

// NewAgent constructs a read-only definition from config. Slice fields and the
// semantic version are copied. Executable Action and Condition implementations
// remain referenced as SPI values; Engine deployment snapshots their
// planner-visible metadata before execution.
func NewAgent(config AgentConfig) *Agent {
	return &Agent{config: config.clone()}
}

func (c AgentConfig) clone() AgentConfig {
	c.Actions = slices.Clone(c.Actions)
	c.Goals = slices.Clone(c.Goals)
	c.Conditions = slices.Clone(c.Conditions)
	c.SnapshotState = slices.Clone(c.SnapshotState)
	return c
}

// Name returns the deployment name of the definition.
func (a *Agent) Name() string {
	if a == nil {
		return ""
	}
	return a.config.Name
}

// Description returns the human-readable purpose of the definition.
func (a *Agent) Description() string {
	if a == nil {
		return ""
	}
	return a.config.Description
}

// Version returns the optional semantic version string.
func (a *Agent) Version() string {
	if a == nil {
		return ""
	}
	return a.config.Version
}

// StuckPolicy returns the recovery policy for a planless process.
func (a *Agent) StuckPolicy() StuckPolicy {
	if a == nil {
		return nil
	}
	return a.config.StuckPolicy
}

// Actions returns a snapshot of the planner-visible action set.
func (a *Agent) Actions() []Action {
	if a == nil {
		return nil
	}
	return slices.Clone(a.config.Actions)
}

// Goals returns a snapshot of the definition's immutable goals.
func (a *Agent) Goals() []*Goal {
	if a == nil {
		return nil
	}
	return slices.Clone(a.config.Goals)
}

// Conditions returns a snapshot of the definition's condition implementations.
func (a *Agent) Conditions() []Condition {
	if a == nil {
		return nil
	}
	return slices.Clone(a.config.Conditions)
}

// SnapshotState returns a snapshot of the agent-owned blackboard schema that is
// independent of action inputs and outputs.
func (a *Agent) SnapshotState() []Binding {
	if a == nil {
		return nil
	}
	return slices.Clone(a.config.SnapshotState)
}

// PlannerName returns the requested planner extension name. Empty means the
// framework default.
func (a *Agent) PlannerName() string {
	if a == nil {
		return ""
	}
	return a.config.PlannerName
}

// Descriptor projects the definition into an inert value suitable for
// selection, policy, and observation boundaries.
func (a *Agent) Descriptor() AgentDescriptor {
	if a == nil {
		return AgentDescriptor{}
	}
	descriptor := AgentDescriptor{
		name:          a.config.Name,
		description:   a.config.Description,
		version:       a.config.Version,
		actions:       make([]ActionDescriptor, len(a.config.Actions)),
		goals:         make([]GoalDescriptor, len(a.config.Goals)),
		conditions:    make([]ConditionDescriptor, len(a.config.Conditions)),
		snapshotState: slices.Clone(a.config.SnapshotState),
		plannerName:   a.config.PlannerName,
	}
	for index, action := range a.config.Actions {
		if !nilvalue.Is(action) {
			if metadata, err := inspectActionMetadata(action); err == nil {
				descriptor.actions[index] = metadata.Descriptor()
			}
		}
	}
	for index, goal := range a.config.Goals {
		descriptor.goals[index] = goal.Descriptor()
	}
	for index, condition := range a.config.Conditions {
		if !nilvalue.Is(condition) {
			if metadata, err := inspectConditionMetadata(condition); err == nil {
				descriptor.conditions[index] = metadata
			}
		}
	}
	return descriptor
}

// Name returns the definition's identity.
func (d AgentDescriptor) Name() string { return d.name }

// Description returns the caller-supplied human-readable purpose.
func (d AgentDescriptor) Description() string { return d.description }

// Version returns the optional semantic version string.
func (d AgentDescriptor) Version() string { return d.version }

// Actions returns independent non-executable action descriptions.
func (d AgentDescriptor) Actions() []ActionDescriptor { return slices.Clone(d.actions) }

// Goals returns independent non-executable goal descriptions.
func (d AgentDescriptor) Goals() []GoalDescriptor { return slices.Clone(d.goals) }

// Conditions returns independent non-executable condition descriptions.
func (d AgentDescriptor) Conditions() []ConditionDescriptor {
	return slices.Clone(d.conditions)
}

// SnapshotState returns an independent snapshot of the declared state schema.
func (d AgentDescriptor) SnapshotState() []Binding {
	return slices.Clone(d.snapshotState)
}

// PlannerName returns the requested planner extension name.
func (d AgentDescriptor) PlannerName() string { return d.plannerName }

// Validate checks all built-in invariants required by deployment: identity,
// unique capabilities, and conservative goal reachability.
//
// Distinct from the [AgentValidator] extension SPI: this is the
// framework's built-in structural check on the entity itself; an
// AgentValidator adds caller-defined deploy-time rules on top.
//
// Independent violations are joined so one deployment attempt reports the
// complete definition problem set. Action and Condition metadata are sampled
// once; a hosted implementation panic becomes an attributed validation error.
func (a *Agent) Validate() error {
	if a == nil {
		return errors.New("agent.Agent.Validate: invalid agent: agent is nil")
	}
	var problems []error
	if a.Name() == "" {
		problems = append(problems, errors.New("agent.Agent.Validate: invalid agent: name is empty"))
	} else if strings.TrimSpace(a.Name()) != a.Name() {
		problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent: name %q has surrounding whitespace", a.Name()))
	}
	if a.Version() != "" {
		if _, err := semver.StrictNewVersion(a.Version()); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: version %q: %w", a.Name(), a.Version(), err))
		}
	}
	if strings.TrimSpace(a.PlannerName()) != a.PlannerName() {
		problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: planner name %q has surrounding whitespace", a.Name(), a.PlannerName()))
	}
	if a.config.StuckPolicy != nil && nilvalue.Is(a.config.StuckPolicy) {
		problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: stuck policy is typed nil", a.Name()))
	}

	actionMetadata := make([]ActionMetadata, len(a.config.Actions))
	actionReadable := make([]bool, len(a.config.Actions))
	actionNames := make([]namedValidationEntry, len(a.config.Actions))
	for index, action := range a.config.Actions {
		if nilvalue.Is(action) {
			actionNames[index].isNil = true
			continue
		}
		metadata, err := inspectActionMetadata(action)
		if err != nil {
			actionNames[index].unreadable = true
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: action at index %d: %w", a.Name(), index, err))
			continue
		}
		actionMetadata[index] = metadata
		actionReadable[index] = true
		actionNames[index].name = metadata.Name
		if err := metadata.validate(); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: action %q: %w", a.Name(), metadata.Name, err))
		}
	}
	goalNames := make([]namedValidationEntry, len(a.config.Goals))
	for index, goal := range a.config.Goals {
		if goal == nil {
			goalNames[index].isNil = true
			continue
		}
		goalNames[index].name = goal.Name()
		if err := goal.validate(); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: goal %q: %w", a.Name(), goal.Name(), err))
		}
	}
	conditionMetadata := make([]ConditionDescriptor, len(a.config.Conditions))
	conditionReadable := make([]bool, len(a.config.Conditions))
	conditionNames := make([]namedValidationEntry, len(a.config.Conditions))
	for index, condition := range a.config.Conditions {
		if nilvalue.Is(condition) {
			conditionNames[index].isNil = true
			continue
		}
		metadata, err := inspectConditionMetadata(condition)
		if err != nil {
			conditionNames[index].unreadable = true
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: condition at index %d: %w", a.Name(), index, err))
			continue
		}
		conditionMetadata[index] = metadata
		conditionReadable[index] = true
		conditionNames[index].name = metadata.Name()
		if cost := metadata.Cost(); math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: condition %q cost %v must be finite and non-negative", a.Name(), metadata.Name(), cost))
		}
	}
	for _, collection := range []struct {
		kind    string
		entries []namedValidationEntry
		require bool
	}{
		{kind: "action", entries: actionNames, require: true},
		{kind: "goal", entries: goalNames, require: true},
		{kind: "condition", entries: conditionNames},
	} {
		if err := a.validateUniqueNamed(collection.kind, collection.entries, collection.require); err != nil {
			problems = append(problems, err)
		}
	}
	seenState := make(map[string]struct{}, len(a.config.SnapshotState))
	for index, binding := range a.config.SnapshotState {
		if err := binding.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: snapshot state[%d]: %w", a.Name(), index, err))
			continue
		}
		if binding.goType == nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: snapshot state[%d] %s must be constructed with NewBinding", a.Name(), index, binding))
			continue
		}
		key := binding.String()
		if _, duplicate := seenState[key]; duplicate {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: duplicate snapshot state %s", a.Name(), binding))
			continue
		}
		seenState[key] = struct{}{}
	}
	problems = append(problems, a.goalReachabilityErrors(actionMetadata, actionReadable, conditionMetadata, conditionReadable)...)
	return errors.Join(problems...)
}

func inspectActionMetadata(action Action) (metadata ActionMetadata, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			metadata = ActionMetadata{}
			err = panicerr.New("Metadata panicked", recovered)
		}
	}()
	return action.Metadata(), nil
}

func inspectConditionMetadata(condition Condition) (metadata ConditionDescriptor, err error) {
	operation := "Name"
	defer func() {
		if recovered := recover(); recovered != nil {
			metadata = ConditionDescriptor{}
			err = panicerr.New(operation+" panicked", recovered)
		}
	}()
	name := condition.Name()
	operation = "Cost"
	return ConditionDescriptor{name: name, cost: condition.Cost()}, nil
}

// goalReachabilityErrors does a conservative one-step producer scan: for
// every condition each goal requires, verify that either an action's
// effects can establish it OR an action's input binding looks like
// it (input bindings are externally supplied via process bindings +
// dual-binding rules). It returns one error per unreachable
// (goal, condition) pair so deploy-time validation can report them
// all together.
//
// Intentionally weaker than running the full planner from empty
// state — the latter falsely rejects agents whose first action's
// precondition is "input binding present", because empty world state
// has no bindings. We accept the false-negative tradeoff so
// legitimate input-driven agents can deploy. Nil or unreadable capabilities
// and nil goals are skipped here; [Agent.Validate] reports those separately.
func (a *Agent) goalReachabilityErrors(
	actions []ActionMetadata,
	actionReadable []bool,
	conditions []ConditionDescriptor,
	conditionReadable []bool,
) []error {
	producible := map[string]struct{}{}
	for index, metadata := range actions {
		if !actionReadable[index] {
			continue
		}
		for key, value := range metadata.Effects {
			if value == True {
				producible[key] = struct{}{}
			}
		}
		for _, input := range metadata.Inputs {
			producible[input.String()] = struct{}{}
		}
	}
	for index, condition := range conditions {
		if conditionReadable[index] && condition.Name() != "" {
			producible[condition.Name()] = struct{}{}
		}
	}
	for _, goal := range a.config.Goals {
		if goal == nil {
			continue
		}
		for _, input := range goal.Inputs() {
			producible[input.String()] = struct{}{}
		}
	}

	var problems []error
	for _, goal := range a.config.Goals {
		if goal == nil {
			continue
		}
		for key, required := range goal.Preconditions() {
			if required != True {
				continue
			}
			if _, ok := producible[key]; !ok {
				problems = append(problems, fmt.Errorf(
					"agent.Agent.Validate: goal %q requires condition %q, but no action produces it",
					goal.Name(), key,
				))
			}
		}
	}
	return problems
}

type namedValidationEntry struct {
	name       string
	isNil      bool
	unreadable bool
}

func (a *Agent) validateUniqueNamed(kind string, entries []namedValidationEntry, require bool) error {
	if require && len(entries) == 0 {
		return fmt.Errorf("agent.Agent.Validate: invalid agent %q: at least one %s is required", a.Name(), kind)
	}
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		switch {
		case entry.isNil:
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: %s at index %d is nil", a.Name(), kind, index)
		case entry.unreadable:
			continue
		case entry.name == "":
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: %s at index %d has empty name", a.Name(), kind, index)
		case strings.TrimSpace(entry.name) != entry.name:
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: %s name %q has surrounding whitespace", a.Name(), kind, entry.name)
		}
		if _, duplicate := seen[entry.name]; duplicate {
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: duplicate %s name %q", a.Name(), kind, entry.name)
		}
		seen[entry.name] = struct{}{}
	}
	return nil
}
