package core

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Masterminds/semver/v3"
)

// AgentConfig is the single input to [NewAgent] — it bundles every piece
// of state the constructor needs (scalar attributes plus the action / goal
// / condition / domain-type / tool-group slices). The DSL [Builder] is a
// thin façade that accumulates fields here and calls [NewAgent] at
// [Builder.Build] time, so callers who already have an AgentConfig in hand
// can skip the Builder entirely.
type AgentConfig struct {
	// Name is the agent's identifier — required, must be unique within
	// a Platform.
	Name string

	// Provider stamps the publisher / vendor.
	Provider string

	// Description is the human-readable summary surfaced in tracing
	// and (when the agent is exposed externally) the LLM prompt.
	Description string

	// Version is the semver tag. Nil falls back to 1.0.0 in
	// [AgentConfig.applyDefaults].
	Version *semver.Version

	// Opaque flags the agent as not-introspectable from the outside.
	Opaque bool

	// StuckHandler is the recovery hook fired when the planner returns
	// no plan. Optional — the default is "transition to StatusStuck".
	StuckHandler StuckHandler

	// Actions are the GOAP-planner-visible actions. At least one
	// action is required for the planner to be useful.
	Actions []Action

	// Goals are the success criteria the planner picks among.
	Goals []*Goal

	// Conditions are user-supplied named predicates the world-state
	// determiner can evaluate alongside the auto-derived ones.
	Conditions []Condition

	// DomainTypes registers planning-relevant types — used when the
	// agent has sealed-style interfaces and the planner needs the
	// parent hierarchy for type-binding lookups.
	DomainTypes []DomainType

	// ToolGroupRequirements declared at agent scope. Per-action
	// requirements live on [ActionMetadata]; the resolver consults
	// both.
	ToolGroupRequirements []ToolGroupRequirement
}

// defaultVersion is the implicit Agent version when AgentConfig.Version is
// nil. Parsed once at package init via [semver.MustParse].
var defaultVersion = semver.MustParse("1.0.0")

// applyDefaults fills in zero-valued fields whose conceptual default is
// non-zero. Mutates the receiver. Idempotent.
func (c *AgentConfig) applyDefaults() {
	if c.Version == nil {
		c.Version = defaultVersion
	}
}

// Agent is the deployable bundle the planner reasons over. The configured
// state is held verbatim via the embedded [AgentConfig]; the trailing
// fields are runtime-only caches that [NewAgent] zero-initialises.
//
// Agent is deliberately small — orchestration knobs live in
// [ProcessOptions], runtime state lives in [AgentProcess].
type Agent struct {
	AgentConfig

	knownConditions     atomic.Pointer[map[string]struct{}]
	knownConditionsOnce sync.Once
}

// NewAgent assembles a fresh agent from cfg. Slice fields are stored by
// reference; callers shouldn't mutate them afterwards. Zero-valued
// scalars are filled by [AgentConfig.applyDefaults].
func NewAgent(cfg AgentConfig) *Agent {
	cfg.applyDefaults()
	return &Agent{AgentConfig: cfg}
}

// KnownConditions enumerates every condition key this agent can refer to —
// the union of action.preconditions/effects keys, goal preconditions, and
// named Condition.Name() values. The world-state determiner asks for this
// list so it can decide what to evaluate during the observe phase.
//
// Result is cached after first call (Agent is immutable post-construction).
func (a *Agent) KnownConditions() map[string]struct{} {
	if cached := a.knownConditions.Load(); cached != nil {
		return *cached
	}

	a.knownConditionsOnce.Do(func() {
		computed := KnownConditions(a.Actions, a.Goals, a.Conditions)
		a.knownConditions.Store(&computed)
	})
	return *a.knownConditions.Load()
}

// ValidateAgent checks structural invariants that must hold for any
// runnable agent: a non-empty name, at least one action, at least one
// goal, and unique action / goal names within the agent. It does NOT
// verify goal reachability — that requires the planner and lives on
// [github.com/Tangerg/lynx/agent/runtime.Platform.Deploy], which can
// reach the configured planner factory.
//
// Returns the first violation found; nil when the agent is well-formed.
// The intent is fail-fast at deploy time rather than at first tick.
func ValidateAgent(a *Agent) error {
	if a == nil {
		return errors.New("core.ValidateAgent: agent is nil")
	}
	if a.Name == "" {
		return errors.New("core.ValidateAgent: agent must have a non-empty Name")
	}
	if len(a.Actions) == 0 {
		return fmt.Errorf("core.ValidateAgent: agent %q has no actions", a.Name)
	}
	if len(a.Goals) == 0 {
		return fmt.Errorf("core.ValidateAgent: agent %q has no goals", a.Name)
	}

	seenActions := make(map[string]struct{}, len(a.Actions))
	for _, action := range a.Actions {
		name := action.Metadata().Name
		if name == "" {
			return fmt.Errorf("core.ValidateAgent: agent %q has an action with empty Name", a.Name)
		}
		if _, dup := seenActions[name]; dup {
			return fmt.Errorf("core.ValidateAgent: agent %q has duplicate action name %q", a.Name, name)
		}
		seenActions[name] = struct{}{}
	}

	seenGoals := make(map[string]struct{}, len(a.Goals))
	for _, goal := range a.Goals {
		if goal == nil {
			return fmt.Errorf("core.ValidateAgent: agent %q has a nil goal", a.Name)
		}
		if goal.Name == "" {
			return fmt.Errorf("core.ValidateAgent: agent %q has a goal with empty Name", a.Name)
		}
		if _, dup := seenGoals[goal.Name]; dup {
			return fmt.Errorf("core.ValidateAgent: agent %q has duplicate goal name %q", a.Name, goal.Name)
		}
		seenGoals[goal.Name] = struct{}{}
	}

	return nil
}

// KnownConditions is the pure builder reused by Agent and
// plan.PlanningSystem caches: union of action precondition / effect keys,
// goal precondition keys, and named-Condition names.
func KnownConditions(actions []Action, goals []*Goal, conditions []Condition) map[string]struct{} {
	out := map[string]struct{}{}

	for _, action := range actions {
		meta := action.Metadata()
		for key := range meta.Preconditions {
			out[key] = struct{}{}
		}
		for key := range meta.Effects {
			out[key] = struct{}{}
		}
	}

	for _, goal := range goals {
		for key := range goal.Preconditions() {
			out[key] = struct{}{}
		}
	}

	for _, cond := range conditions {
		out[cond.Name()] = struct{}{}
	}
	return out
}

