package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
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

	// Version is an optional semantic version. Empty means the definition is
	// unversioned; durable runtimes then require a Host BuildID. Validation is
	// owned by Agent so callers do not need semver types in configuration.
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

// PlannerName returns the requested planner extension name. Empty means the
// framework default.
func (a *Agent) PlannerName() string {
	if a == nil {
		return ""
	}
	return a.config.PlannerName
}

// Validate checks all built-in invariants required by deployment: identity,
// unique capabilities, retry safety, and conservative goal reachability.
//
// Distinct from the [AgentValidator] extension SPI: this is the
// framework's built-in structural check on the entity itself; an
// AgentValidator adds caller-defined deploy-time rules on top.
//
// Independent violations are joined so one deployment attempt reports the
// complete definition problem set.
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
		if _, err := semver.NewVersion(a.Version()); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: version %q: %w", a.Name(), a.Version(), err))
		}
	}

	type namedCollection struct {
		kind    string
		name    func(int) (string, bool) // (name, isNil)
		count   int
		require bool
	}
	for _, collection := range []namedCollection{
		{"action", func(index int) (string, bool) {
			if a.config.Actions[index] == nil {
				return "", true
			}
			return a.config.Actions[index].Metadata().Name, false
		}, len(a.config.Actions), true},
		{"goal", func(index int) (string, bool) {
			if a.config.Goals[index] == nil {
				return "", true
			}
			return a.config.Goals[index].Name(), false
		}, len(a.config.Goals), true},
		{"condition", func(index int) (string, bool) {
			if a.config.Conditions[index] == nil {
				return "", true
			}
			return a.config.Conditions[index].Name(), false
		}, len(a.config.Conditions), false},
	} {
		if err := a.validateUniqueNamed(collection.kind, collection.count, collection.name, collection.require); err != nil {
			problems = append(problems, err)
		}
	}
	for _, action := range a.config.Actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		if err := metadata.validate(); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: action %q: %w", a.Name(), metadata.Name, err))
		}
	}
	for _, goal := range a.config.Goals {
		if goal == nil {
			continue
		}
		if err := goal.validate(); err != nil {
			problems = append(problems, fmt.Errorf("agent.Agent.Validate: invalid agent %q: goal %q: %w", a.Name(), goal.Name(), err))
		}
	}
	problems = append(problems, a.goalReachabilityErrors()...)
	return errors.Join(problems...)
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
// legitimate input-driven agents can deploy. Nil actions/goals are
// skipped here; [Agent.Validate] reports those separately.
func (a *Agent) goalReachabilityErrors() []error {
	if a == nil {
		return nil
	}
	producible := map[string]struct{}{}
	for _, action := range a.config.Actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		for key, value := range metadata.Effects {
			if value == True {
				producible[key] = struct{}{}
			}
		}
		for _, input := range metadata.Inputs {
			producible[input.String()] = struct{}{}
		}
	}
	for _, condition := range a.config.Conditions {
		if condition != nil && condition.Name() != "" {
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

func (a *Agent) validateUniqueNamed(
	kind string,
	count int,
	nameAt func(int) (name string, isNil bool),
	require bool,
) error {
	if require && count == 0 {
		return fmt.Errorf("agent.Agent.Validate: invalid agent %q: at least one %s is required", a.Name(), kind)
	}
	seen := make(map[string]struct{}, count)
	for i := range count {
		name, isNil := nameAt(i)
		switch {
		case isNil:
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: %s at index %d is nil", a.Name(), kind, i)
		case name == "":
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: %s at index %d has empty name", a.Name(), kind, i)
		case strings.TrimSpace(name) != name:
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: %s name %q has surrounding whitespace", a.Name(), kind, name)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("agent.Agent.Validate: invalid agent %q: duplicate %s name %q", a.Name(), kind, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
