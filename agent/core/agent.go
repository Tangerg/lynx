package core

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Masterminds/semver/v3"
)

// AgentConfig is the input to [NewAgent]: the scalar attributes plus
// the action / goal / condition slices. The DSL [Builder] is a thin
// façade that accumulates fields here and calls [NewAgent] on
// [Builder.Build].
type AgentConfig struct {
	// Name is the agent's identifier — required, unique within a
	// Platform.
	Name string

	// Description is the human-readable summary surfaced in tracing
	// and (when the agent is exposed externally) the LLM prompt.
	Description string

	// Version is the semver tag. Nil → 1.0.0.
	Version *semver.Version

	// StuckHandler is the recovery hook fired when the planner
	// returns no plan. Nil → transition to StatusStuck.
	StuckHandler StuckHandler

	// Actions are the planner-visible actions. ≥1 required.
	Actions []Action

	// Goals are the success criteria the planner picks among.
	Goals []*Goal

	// Conditions are user-supplied named predicates the world-state
	// determiner can evaluate alongside the auto-derived ones.
	Conditions []Condition

	// PlannerName selects which planner the runtime uses for this
	// agent. It must match the [Extension.Name] of a planner
	// registered on the platform (or via process-scope
	// [ProcessOptions.Extensions]). Empty → framework default
	// ("goap"). Built-in names: "goap", "htn", "reactive".
	PlannerName string
}

// defaultVersion is the implicit Agent version when
// AgentConfig.Version is nil.
var defaultVersion = semver.MustParse("1.0.0")

// Agent is the deployable bundle the planner reasons over. The configured
// state is held verbatim via the embedded [AgentConfig]; the trailing
// field is a runtime-only cache populated lazily on first use.
//
// Agent is deliberately small — orchestration knobs live in
// [ProcessOptions], runtime state lives in [AgentProcess].
type Agent struct {
	AgentConfig

	// knownConditions is the lazily-computed condition-key cache.
	// Initialised by [NewAgent] via [sync.OnceValue]; subsequent
	// [Agent.KnownConditions] calls are a single function call.
	knownConditions func() map[string]struct{}
}

// NewAgent assembles a fresh agent from config. Slice fields are
// stored by reference; callers shouldn't mutate them afterwards.
// nil config is treated as a zero-value config (no actions / goals).
func NewAgent(config AgentConfig) *Agent {
	if config.Version == nil {
		config.Version = defaultVersion
	}
	a := &Agent{AgentConfig: config}
	a.knownConditions = sync.OnceValue(func() map[string]struct{} {
		return KnownConditions(a.Actions, a.Goals, a.Conditions)
	})
	return a
}

// KnownConditions enumerates every condition key this agent can refer to —
// the union of action.preconditions/effects keys, goal preconditions, and
// named Condition.Name() values. The world-state determiner asks for this
// list so it can decide what to evaluate during the observe phase.
//
// Result is cached after first call (Agent is immutable post-construction).
func (a *Agent) KnownConditions() map[string]struct{} {
	return a.knownConditions()
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
		return errors.New("invalid agent: agent is nil")
	}
	if a.Name == "" {
		return errors.New("invalid agent: name is empty")
	}

	type item struct {
		kind    string
		name    func(int) (string, bool) // (name, isNil)
		count   int
		require bool
	}
	for _, it := range []item{
		{"action", func(i int) (string, bool) {
			if a.Actions[i] == nil {
				return "", true
			}
			return a.Actions[i].Metadata().Name, false
		}, len(a.Actions), true},
		{"goal", func(i int) (string, bool) {
			if a.Goals[i] == nil {
				return "", true
			}
			return a.Goals[i].Name, false
		}, len(a.Goals), true},
		{"condition", func(i int) (string, bool) {
			if a.Conditions[i] == nil {
				return "", true
			}
			return a.Conditions[i].Name(), false
		}, len(a.Conditions), false},
	} {
		if err := validateUniqueNamed(a.Name, it.kind, it.count, it.name, it.require); err != nil {
			return err
		}
	}
	return nil
}

// validateUniqueNamed checks one named-element slice for "≥1 entry
// when require, no nils, no empty names, no duplicate names". Lifts
// the dedup loop the three (action / goal / condition) callsites used
// to copy-paste.
func validateUniqueNamed(
	agentName, kind string,
	count int,
	nameAt func(int) (name string, isNil bool),
	require bool,
) error {
	if require && count == 0 {
		return fmt.Errorf("invalid agent %q: at least one %s is required", agentName, kind)
	}
	seen := make(map[string]struct{}, count)
	for i := range count {
		name, isNil := nameAt(i)
		switch {
		case isNil:
			return fmt.Errorf("invalid agent %q: %s at index %d is nil", agentName, kind, i)
		case name == "":
			return fmt.Errorf("invalid agent %q: %s at index %d has empty name", agentName, kind, i)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("invalid agent %q: duplicate %s name %q", agentName, kind, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// KnownConditions is the pure builder reused by Agent and
// planning.System caches: union of action precondition / effect keys,
// goal precondition keys, and named-Condition names.
func KnownConditions(actions []Action, goals []*Goal, conditions []Condition) map[string]struct{} {
	out := map[string]struct{}{}

	for _, action := range actions {
		if action == nil {
			continue
		}
		meta := action.Metadata()
		for key := range meta.Preconditions {
			out[key] = struct{}{}
		}
		for key := range meta.Effects {
			out[key] = struct{}{}
		}
	}

	for _, goal := range goals {
		if goal == nil {
			continue
		}
		for key := range goal.Preconditions() {
			out[key] = struct{}{}
		}
	}

	for _, condition := range conditions {
		if condition == nil {
			continue
		}
		out[condition.Name()] = struct{}{}
	}
	return out
}
