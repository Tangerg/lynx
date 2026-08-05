package planning

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// Domain is an immutable planning-definition snapshot, detached from agent
// identity so a planner can reason over any subset. Action execution and
// condition evaluation remain delegated to the supplied capabilities.
type Domain struct {
	actions        []core.Action
	goals          []*core.Goal
	conditions     []core.Condition
	conditionRefs  []ConditionRef
	conditionByKey map[string]ConditionRef
	conditionOrder map[string]int
}

// domainAction owns the metadata snapshot a Domain exposes while delegating
// execution to the capability supplied at construction.
type domainAction struct {
	delegate core.Action
	metadata core.ActionMetadata
}

func (a domainAction) Metadata() core.ActionMetadata { return a.metadata.Clone() }

func (a domainAction) Execute(ctx context.Context, process *core.ProcessContext) (core.ActionStatus, error) {
	return a.delegate.Execute(ctx, process)
}

// domainCondition freezes planner-visible identity and cost while preserving
// the supplied evaluator as the only executable capability.
type domainCondition struct {
	delegate       core.Condition
	name           string
	evaluationCost float64
}

func (c domainCondition) Name() string            { return c.name }
func (c domainCondition) EvaluationCost() float64 { return c.evaluationCost }

func (c domainCondition) Evaluate(ctx context.Context, environment *core.ConditionEnv) core.Truth {
	return c.delegate.Evaluate(ctx, environment)
}

// ConditionSourceKind identifies how the runtime obtains a condition's current value.
type ConditionSourceKind uint8

const (
	ConditionFact ConditionSourceKind = iota
	ConditionBinding
	ConditionActionSuccess
	ConditionEvaluator
)

// Valid reports whether k identifies a framework-defined condition source.
func (k ConditionSourceKind) Valid() bool {
	return k >= ConditionFact && k <= ConditionEvaluator
}

func (k ConditionSourceKind) String() string {
	switch k {
	case ConditionFact:
		return "fact"
	case ConditionBinding:
		return "binding"
	case ConditionActionSuccess:
		return "action success"
	case ConditionEvaluator:
		return "evaluator"
	default:
		return fmt.Sprintf("unknown condition kind %d", k)
	}
}

// ConditionRef identifies one planner-visible condition and its value source.
// Binding is populated only when Source is [ConditionBinding].
type ConditionRef struct {
	Key     string
	Source  ConditionSourceKind
	Binding core.Binding
	// EvaluationCost is the relative evaluation cost when Source is
	// [ConditionEvaluator]. Other condition sources have zero evaluation cost.
	EvaluationCost float64
}

type conditionSource struct {
	ref    ConditionRef
	origin string
}

// conditionSources indexes every condition a domain declares, keyed the way the
// planner will look it up. It exists so a second declaration of the same key can
// be judged against the first: two declarations agree only when both are
// bindings, since one binding may legitimately be produced by an action and
// required by a goal. Anything else is a domain whose planner would read one
// key as two different things.
//
// Order is the order keys were first declared, kept so a domain compiles to the
// same ref sequence on every run.
type conditionSources struct {
	byKey map[string]conditionSource
	order []string
}

func newConditionSources() conditionSources {
	return conditionSources{byKey: map[string]conditionSource{}}
}

func (s *conditionSources) declare(ref ConditionRef, origin string) error {
	if existing, ok := s.byKey[ref.Key]; ok {
		if existing.ref.Source == ref.Source && ref.Source == ConditionBinding {
			return nil
		}
		return fmt.Errorf(
			"planning.NewDomain: condition %q has conflicting %s (%s) and %s (%s) sources",
			ref.Key,
			existing.ref.Source,
			existing.origin,
			ref.Source,
			origin,
		)
	}
	s.byKey[ref.Key] = conditionSource{ref: ref, origin: origin}
	s.order = append(s.order, ref.Key)
	return nil
}

// refFor returns the declared ref for key, or a plain fact. A key reached only
// through a precondition or effect has no declaring action, goal, or evaluator:
// it is a fact the world state supplies.
func (s conditionSources) refFor(key string) ConditionRef {
	if source, ok := s.byKey[key]; ok {
		return source.ref
	}
	return ConditionRef{Key: key, Source: ConditionFact}
}

// NewDomain constructs a domain from explicit slices. Pass nil for any unused
// dimension. It rejects nil slice members and condition keys claimed by
// incompatible value sources.
func NewDomain(actions []core.Action, goals []*core.Goal, conditions []core.Condition) (*Domain, error) {
	for index, action := range actions {
		if nilvalue.Is(action) {
			return nil, fmt.Errorf("planning.NewDomain: action[%d] is nil", index)
		}
	}
	for index, goal := range goals {
		if goal == nil {
			return nil, fmt.Errorf("planning.NewDomain: goal[%d] is nil", index)
		}
	}
	for index, condition := range conditions {
		if nilvalue.Is(condition) {
			return nil, fmt.Errorf("planning.NewDomain: condition[%d] is nil", index)
		}
	}

	snapshottedActions := make([]core.Action, len(actions))
	for index, action := range actions {
		snapshot, err := snapshotDomainAction(action)
		if err != nil {
			return nil, fmt.Errorf("planning.NewDomain: action[%d]: %w", index, err)
		}
		snapshottedActions[index] = snapshot
	}
	snapshottedConditions := make([]core.Condition, len(conditions))
	for index, condition := range conditions {
		snapshot, err := snapshotDomainCondition(condition)
		if err != nil {
			return nil, fmt.Errorf("planning.NewDomain: condition[%d]: %w", index, err)
		}
		snapshottedConditions[index] = snapshot
	}
	domain := &Domain{
		actions:    snapshottedActions,
		goals:      slices.Clone(goals),
		conditions: snapshottedConditions,
	}
	refs, err := domain.computeConditionRefs()
	if err != nil {
		return nil, err
	}
	domain.conditionRefs = refs
	domain.conditionByKey = make(map[string]ConditionRef, len(refs))
	domain.conditionOrder = make(map[string]int, len(refs))
	for index, ref := range refs {
		domain.conditionByKey[ref.Key] = ref
		domain.conditionOrder[ref.Key] = index
	}
	return domain, nil
}

func snapshotDomainAction(action core.Action) (snapshot core.Action, err error) {
	if owned, ok := action.(domainAction); ok {
		return owned, nil
	}
	metadata, err := inspectActionMetadata(action)
	if err != nil {
		return nil, err
	}
	return domainAction{delegate: action, metadata: metadata.Clone()}, nil
}

func inspectActionMetadata(action core.Action) (metadata core.ActionMetadata, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			metadata = core.ActionMetadata{}
			err = panicerr.New("Action.Metadata panicked", recovered)
		}
	}()
	return action.Metadata(), nil
}

func snapshotDomainCondition(condition core.Condition) (snapshot core.Condition, err error) {
	if owned, ok := condition.(domainCondition); ok {
		return owned, nil
	}
	operation := "Name"
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot = nil
			err = panicerr.New("Condition."+operation+" panicked", recovered)
		}
	}()
	name := condition.Name()
	operation = "EvaluationCost"
	return domainCondition{delegate: condition, name: name, evaluationCost: condition.EvaluationCost()}, nil
}

// Actions returns a snapshot of the available actions.
func (d *Domain) Actions() []core.Action {
	if d == nil {
		return nil
	}
	return slices.Clone(d.actions)
}

func (d *Domain) action(name string) (core.Action, bool) {
	for _, action := range d.actions {
		if action.Metadata().Name == name {
			return action, true
		}
	}
	return nil, false
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

// ConditionRefs enumerates all conditions reachable through the domain.
// Iteration is deterministic: action and goal declaration order first, with
// map-backed keys sorted within each declaration, followed by named conditions.
func (d *Domain) ConditionRefs() iter.Seq[ConditionRef] {
	if d == nil {
		return slices.Values([]ConditionRef(nil))
	}
	return slices.Values(d.conditionRefs)
}

// ConditionRef returns the planner-visible source metadata for key.
func (d *Domain) ConditionRef(key string) (ConditionRef, bool) {
	if d == nil {
		return ConditionRef{}, false
	}
	return d.conditionRef(key)
}

// computeConditionRefs validates the domain and compiles its condition
// vocabulary. Declaring and ordering are separate passes on purpose: every
// conflict has to be known before any ref is emitted, or the emitted order would
// depend on which conflict happened to be reached first.
func (d *Domain) computeConditionRefs() ([]ConditionRef, error) {
	sources, err := d.declareConditionSources()
	if err != nil {
		return nil, err
	}
	return d.orderConditionRefs(sources), nil
}

// declareConditionSources validates every action, goal, and condition and records
// the condition keys each one declares.
func (d *Domain) declareConditionSources() (conditionSources, error) {
	sources := newConditionSources()
	for _, action := range d.actions {
		metadata := action.Metadata()
		if strings.TrimSpace(metadata.Name) == "" || strings.TrimSpace(metadata.Name) != metadata.Name {
			return sources, fmt.Errorf("planning.NewDomain: action name %q must be non-empty without surrounding whitespace", metadata.Name)
		}
		if err := metadata.Preconditions.Validate(); err != nil {
			return sources, fmt.Errorf("planning.NewDomain: action %q preconditions: %w", metadata.Name, err)
		}
		if err := metadata.Effects.Validate(); err != nil {
			return sources, fmt.Errorf("planning.NewDomain: action %q effects: %w", metadata.Name, err)
		}
		origin := "action " + metadata.Name
		for _, binding := range slices.Concat(metadata.Inputs, metadata.Outputs) {
			if err := binding.Validate(); err != nil {
				return sources, fmt.Errorf("planning.NewDomain: action %q binding: %w", metadata.Name, err)
			}
			binding = binding.Canonical()
			if err := sources.declare(ConditionRef{Key: binding.String(), Source: ConditionBinding, Binding: binding}, origin); err != nil {
				return sources, err
			}
		}
		if err := sources.declare(ConditionRef{Key: metadata.SuccessCondition(), Source: ConditionActionSuccess}, origin); err != nil {
			return sources, err
		}
	}
	for _, goal := range d.goals {
		if strings.TrimSpace(goal.Name()) == "" || strings.TrimSpace(goal.Name()) != goal.Name() {
			return sources, fmt.Errorf("planning.NewDomain: goal name %q must be non-empty without surrounding whitespace", goal.Name())
		}
		if err := goal.Requirements().Validate(); err != nil {
			return sources, fmt.Errorf("planning.NewDomain: goal %q requirements: %w", goal.Name(), err)
		}
		origin := "goal " + goal.Name()
		for _, binding := range goal.RequiredBindings() {
			if err := binding.Validate(); err != nil {
				return sources, fmt.Errorf("planning.NewDomain: goal %q binding: %w", goal.Name(), err)
			}
			binding = binding.Canonical()
			if err := sources.declare(ConditionRef{Key: binding.String(), Source: ConditionBinding, Binding: binding}, origin); err != nil {
				return sources, err
			}
		}
	}
	for _, condition := range d.conditions {
		name := condition.Name()
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
			return sources, fmt.Errorf("planning.NewDomain: condition name %q must be non-empty without surrounding whitespace", name)
		}
		if cost := condition.EvaluationCost(); math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
			return sources, fmt.Errorf("planning.NewDomain: condition %q evaluation cost %v must be finite and non-negative", name, cost)
		}
		if err := sources.declare(ConditionRef{Key: name, Source: ConditionEvaluator, EvaluationCost: condition.EvaluationCost()}, "condition "+name); err != nil {
			return sources, err
		}
	}
	return sources, nil
}

// orderConditionRefs emits the vocabulary in the order a planner meets it:
// action preconditions and effects, then goal requirements, then evaluators,
// then anything declared but not yet reached. Keys inside each collection are
// sorted, so one domain always compiles to one sequence.
func (d *Domain) orderConditionRefs(sources conditionSources) []ConditionRef {
	seen := map[string]struct{}{}
	var refs []ConditionRef
	appendKey := func(key string) {
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		refs = append(refs, sources.refFor(key))
	}
	for _, action := range d.actions {
		metadata := action.Metadata()
		for _, key := range slices.Sorted(maps.Keys(metadata.Preconditions)) {
			appendKey(key)
		}
		for _, key := range slices.Sorted(maps.Keys(metadata.Effects)) {
			appendKey(key)
		}
	}
	for _, goal := range d.goals {
		for _, key := range slices.Sorted(maps.Keys(goal.Requirements())) {
			appendKey(key)
		}
	}
	for _, condition := range d.conditions {
		appendKey(condition.Name())
	}
	for _, key := range sources.order {
		appendKey(key)
	}
	return refs
}
