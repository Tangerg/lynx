package core

import (
	"sync"
	"sync/atomic"
)

// Agent is the deployable bundle: a name, a version, and the capability set
// (actions / goals / conditions) that the planner reasons over. It's
// deliberately small — orchestration knobs live in ProcessOptions, runtime
// state lives in AgentProcess.
type Agent struct {
	Name         string
	Provider     string
	Version      Semver
	Description  string
	Actions      []Action
	Goals        []*Goal
	Conditions   []Condition
	StuckHandler StuckHandler
	Opaque       bool
	DomainTypes  []DomainType

	// ToolGroupRequirements declared at agent scope. Per-action requirements
	// live on ActionMetadata; the resolver consults both.
	ToolGroupRequirements []ToolGroupRequirement

	knownConditions     atomic.Pointer[map[string]struct{}]
	knownConditionsOnce sync.Once
}

// AgentMeta is the small immutable header used by NewAgent. Separated from
// the full Agent struct so DSL code can build the body incrementally.
type AgentMeta struct {
	Name         string
	Provider     string
	Description  string
	Version      Semver
	Opaque       bool
	StuckHandler StuckHandler
}

// NewAgent assembles a fresh agent. Inputs are stored by reference; callers
// shouldn't mutate the slices afterward.
func NewAgent(meta AgentMeta, actions []Action, goals []*Goal, conditions []Condition) *Agent {
	return &Agent{
		Name:         meta.Name,
		Provider:     meta.Provider,
		Version:      meta.Version,
		Description:  meta.Description,
		Actions:      actions,
		Goals:        goals,
		Conditions:   conditions,
		StuckHandler: meta.StuckHandler,
		Opaque:       meta.Opaque,
	}
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
		computed := computeKnownConditions(a.Actions, a.Goals, a.Conditions)
		a.knownConditions.Store(&computed)
	})
	return *a.knownConditions.Load()
}

// computeKnownConditions is the pure builder used by both Agent and
// PlanningSystem caches.
func computeKnownConditions(actions []Action, goals []*Goal, conditions []Condition) map[string]struct{} {
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

// FindAction returns the named action (or nil, false). O(n) — the action
// list is short in practice.
func (a *Agent) FindAction(name string) (Action, bool) {
	for _, action := range a.Actions {
		if action.Metadata().Name == name {
			return action, true
		}
	}
	return nil, false
}

// FindGoal mirrors FindAction for goals.
func (a *Agent) FindGoal(name string) (*Goal, bool) {
	for _, goal := range a.Goals {
		if goal.Name == name {
			return goal, true
		}
	}
	return nil, false
}

// WithSingleGoal returns a copy that retains only the named goal — used
// when running an agent toward one specific objective rather than letting
// the planner choose.
func (a *Agent) WithSingleGoal(goal *Goal) *Agent {
	clone := a.shallowClone()
	clone.Goals = []*Goal{goal}
	return clone
}

// Clone makes a defensive copy. The action/goal/condition slices are shared
// since their contents are themselves immutable; only the agent envelope
// gets duplicated.
func (a *Agent) Clone() *Agent { return a.shallowClone() }

func (a *Agent) shallowClone() *Agent {
	return &Agent{
		Name:                  a.Name,
		Provider:              a.Provider,
		Version:               a.Version,
		Description:           a.Description,
		Actions:               a.Actions,
		Goals:                 a.Goals,
		Conditions:            a.Conditions,
		StuckHandler:          a.StuckHandler,
		Opaque:                a.Opaque,
		DomainTypes:           a.DomainTypes,
		ToolGroupRequirements: a.ToolGroupRequirements,
	}
}
