package planning

import (
	"fmt"
	"iter"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// Domain is an immutable capability set passed to a planner, detached from
// agent identity so a planner can reason over any subset.
type Domain struct {
	actions       []core.Action
	goals         []*core.Goal
	conditions    []core.Condition
	conditionRefs []ConditionRef
}

// ConditionKind identifies how the runtime obtains a condition's current value.
type ConditionKind uint8

const (
	ConditionFact ConditionKind = iota
	ConditionBinding
	ConditionActionRun
	ConditionEvaluator
)

// Valid reports whether k identifies a framework-defined condition source.
func (k ConditionKind) Valid() bool {
	return k >= ConditionFact && k <= ConditionEvaluator
}

func (k ConditionKind) String() string {
	switch k {
	case ConditionFact:
		return "fact"
	case ConditionBinding:
		return "binding"
	case ConditionActionRun:
		return "action run"
	case ConditionEvaluator:
		return "evaluator"
	default:
		return fmt.Sprintf("unknown condition kind %d", k)
	}
}

// ConditionRef identifies one planner-visible condition and its value source.
// Binding is populated only when Kind is [ConditionBinding].
type ConditionRef struct {
	Key     string
	Kind    ConditionKind
	Binding core.Binding
}

type conditionSource struct {
	ref    ConditionRef
	origin string
}

// NewDomain constructs a domain from explicit slices. Pass nil for any unused
// dimension. It rejects condition keys claimed by incompatible value sources.
func NewDomain(actions []core.Action, goals []*core.Goal, conditions []core.Condition) (*Domain, error) {
	domain := &Domain{
		actions:    slices.Clone(actions),
		goals:      slices.Clone(goals),
		conditions: slices.Clone(conditions),
	}
	refs, err := domain.computeConditionRefs()
	if err != nil {
		return nil, err
	}
	domain.conditionRefs = refs
	return domain, nil
}

// Actions returns a snapshot of the available actions.
func (d *Domain) Actions() []core.Action {
	if d == nil {
		return nil
	}
	return slices.Clone(d.actions)
}

// Goals returns a snapshot of the candidate goals.
func (d *Domain) Goals() []*core.Goal {
	if d == nil {
		return nil
	}
	return slices.Clone(d.goals)
}

// Conditions returns a snapshot of the named condition implementations.
func (d *Domain) Conditions() []core.Condition {
	if d == nil {
		return nil
	}
	return slices.Clone(d.conditions)
}

// DomainForAgent builds a planning domain out of an agent's capability set —
// convenience for the runtime which wires planner ↔ agent.
func DomainForAgent(agent *core.Agent) (*Domain, error) {
	if agent == nil {
		return NewDomain(nil, nil, nil)
	}
	return NewDomain(agent.Actions(), agent.Goals(), agent.Conditions())
}

// DomainForAgents unions the capability sets of multiple agents into a single
// planning domain — joint planning across agent boundaries. The resulting domain carries the concatenation of every
// agent's actions, goals, and conditions; the planner reasons over the
// whole union and may pick a path that crosses agent boundaries.
//
// Name uniqueness across the input agents is the caller's
// responsibility — the planner does not deduplicate. Nil entries are
// skipped so callers can pass partially-populated slices without
// guarding.
func DomainForAgents(agents []*core.Agent) (*Domain, error) {
	var (
		actions    []core.Action
		goals      []*core.Goal
		conditions []core.Condition
	)
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		actions = append(actions, agent.Actions()...)
		goals = append(goals, agent.Goals()...)
		conditions = append(conditions, agent.Conditions()...)
	}
	return NewDomain(actions, goals, conditions)
}

// KnownConditions enumerates all conditions reachable through the domain.
// Iteration is deterministic: action and goal declaration order first, with
// map-backed keys sorted within each declaration, followed by named conditions.
func (d *Domain) KnownConditions() iter.Seq[ConditionRef] {
	if d == nil {
		return slices.Values([]ConditionRef(nil))
	}
	return slices.Values(d.conditionRefs)
}

func (d *Domain) computeConditionRefs() ([]ConditionRef, error) {
	sources := make(map[string]conditionSource)
	var sourceOrder []string
	register := func(ref ConditionRef, origin string) error {
		if existing, ok := sources[ref.Key]; ok {
			if existing.ref.Kind == ref.Kind && ref.Kind == ConditionBinding {
				return nil
			}
			return fmt.Errorf(
				"planning.NewDomain: condition %q has conflicting %s (%s) and %s (%s) sources",
				ref.Key,
				existing.ref.Kind,
				existing.origin,
				ref.Kind,
				origin,
			)
		}
		sources[ref.Key] = conditionSource{ref: ref, origin: origin}
		sourceOrder = append(sourceOrder, ref.Key)
		return nil
	}

	for _, action := range d.actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		for _, binding := range slices.Concat(metadata.Inputs, metadata.Outputs) {
			binding = binding.Canonical()
			if err := register(ConditionRef{Key: binding.String(), Kind: ConditionBinding, Binding: binding}, "action "+metadata.Name); err != nil {
				return nil, err
			}
		}
		if err := register(ConditionRef{Key: metadata.RunCondition(), Kind: ConditionActionRun}, "action "+metadata.Name); err != nil {
			return nil, err
		}
	}
	for _, goal := range d.goals {
		if goal == nil {
			continue
		}
		for _, binding := range goal.Inputs() {
			binding = binding.Canonical()
			if err := register(ConditionRef{Key: binding.String(), Kind: ConditionBinding, Binding: binding}, "goal "+goal.Name()); err != nil {
				return nil, err
			}
		}
	}
	for _, condition := range d.conditions {
		if condition == nil {
			continue
		}
		if err := register(ConditionRef{Key: condition.Name(), Kind: ConditionEvaluator}, "condition "+condition.Name()); err != nil {
			return nil, err
		}
	}

	seen := map[string]struct{}{}
	var refs []ConditionRef
	appendCondition := func(key string) {
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		if source, ok := sources[key]; ok {
			refs = append(refs, source.ref)
			return
		}
		refs = append(refs, ConditionRef{Key: key, Kind: ConditionFact})
	}
	for _, action := range d.actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		for _, key := range slices.Sorted(maps.Keys(metadata.Preconditions)) {
			appendCondition(key)
		}
		for _, key := range slices.Sorted(maps.Keys(metadata.Effects)) {
			appendCondition(key)
		}
	}
	for _, goal := range d.goals {
		if goal == nil {
			continue
		}
		for _, key := range slices.Sorted(maps.Keys(goal.Preconditions())) {
			appendCondition(key)
		}
	}
	for _, condition := range d.conditions {
		if condition != nil {
			appendCondition(condition.Name())
		}
	}
	for _, key := range sourceOrder {
		appendCondition(key)
	}
	return refs, nil
}
