package planning

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// ConditionResolver obtains the current truth of a named evaluator-backed
// condition. The planning domain decides which conditions are needed and in
// which cost order; the resolver owns observation and per-pass caching.
// Resolve must be safe to call more than once for the same name and return one
// stable observation throughout a planning pass. ResolvedConditions returns a
// defensive snapshot of every successful observation made in that pass.
type ConditionResolver interface {
	Resolve(ctx context.Context, name string) (core.Truth, error)
	ResolvedConditions() core.ConditionSet
}

// conditionRequirement is one evaluator-backed requirement whose current
// truth is still unknown. Cost exists only to choose observation order; it is
// never mixed with action cost or plan value.
type conditionRequirement struct {
	key      string
	required core.Truth
	cost     float64
	order    int
}

// Satisfies reports whether state meets every requirement. Evaluator-backed
// Unknown values are resolved only when all already-known requirements match,
// from lowest to highest evaluation cost. This lets a cheap mismatch
// short-circuit the rest of the conjunction. Equal costs preserve the Domain's
// deterministic condition order.
func (d *Domain) Satisfies(
	ctx context.Context,
	state core.WorldState,
	requirements core.ConditionSet,
	resolver ConditionResolver,
) (bool, error) {
	switch {
	case d == nil:
		return false, errors.New("planning.Domain.Satisfies: domain is nil")
	case nilvalue.Is(state):
		return false, errors.New("planning.Domain.Satisfies: world state is nil")
	}
	values := state.Conditions()
	pending, matches := d.inspectRequirements(values, nil, requirements, !nilvalue.Is(resolver))
	if !matches {
		return false, nil
	}
	slices.SortFunc(pending, compareConditionRequirements)
	for _, requirement := range pending {
		truth, err := resolveCondition(ctx, resolver, requirement.key)
		if err != nil {
			return false, fmt.Errorf("planning.Domain.Satisfies: %w", err)
		}
		if truth != requirement.required {
			return false, nil
		}
	}
	return true, nil
}

// Unsatisfied returns the requirements state does not currently meet. Unlike
// Satisfies, it resolves every evaluator-backed requirement because callers
// need the complete difference set rather than a yes/no answer. Resolution
// still proceeds from lower to higher cost with deterministic tie ordering.
func (d *Domain) Unsatisfied(
	ctx context.Context,
	state core.WorldState,
	requirements core.ConditionSet,
	resolver ConditionResolver,
) (core.ConditionSet, error) {
	switch {
	case d == nil:
		return nil, errors.New("planning.Domain.Unsatisfied: domain is nil")
	case nilvalue.Is(state):
		return nil, errors.New("planning.Domain.Unsatisfied: world state is nil")
	}
	values := state.Conditions()
	if nilvalue.Is(resolver) {
		return values.Unsatisfied(requirements), nil
	}

	pending := make([]conditionRequirement, 0, len(requirements))
	for _, key := range slices.Sorted(maps.Keys(requirements)) {
		required := requirements[key]
		if values[key] == required || values[key] != core.Unknown {
			continue
		}
		ref, ok := d.conditionRef(key)
		if !ok || ref.Kind != ConditionEvaluator {
			continue
		}
		pending = append(pending, conditionRequirement{
			key:      key,
			required: required,
			cost:     ref.Cost,
			order:    d.conditionOrder[key],
		})
	}
	slices.SortFunc(pending, compareConditionRequirements)
	for _, requirement := range pending {
		truth, err := resolveCondition(ctx, resolver, requirement.key)
		if err != nil {
			return nil, fmt.Errorf("planning.Domain.Unsatisfied: %w", err)
		}
		values[requirement.key] = truth
	}
	return values.Unsatisfied(requirements), nil
}

// ApplicableActions returns the domain actions in actions whose preconditions
// hold in state. Inputs are resolved to the Domain's immutable action snapshots
// by name; an action outside the domain is rejected. Evaluator-backed Unknown
// values are resolved globally from lowest to highest cost. A resolved mismatch
// removes every affected candidate before a more expensive condition is
// considered.
func (d *Domain) ApplicableActions(
	ctx context.Context,
	state core.WorldState,
	actions []core.Action,
	resolver ConditionResolver,
) ([]core.Action, error) {
	switch {
	case d == nil:
		return nil, errors.New("planning.Domain.ApplicableActions: domain is nil")
	case nilvalue.Is(state):
		return nil, errors.New("planning.Domain.ApplicableActions: world state is nil")
	}

	candidates, err := d.canonicalActionCandidates(actions)
	if err != nil {
		return nil, fmt.Errorf("planning.Domain.ApplicableActions: %w", err)
	}
	values := state.Conditions()
	resolved := make(core.ConditionSet)
	for {
		applicable := make([]core.Action, 0, len(candidates))
		pendingByKey := make(map[string]conditionRequirement)
		for _, action := range candidates {
			metadata := action.Metadata()
			pending, matches := d.inspectRequirements(values, resolved, metadata.Preconditions, !nilvalue.Is(resolver))
			if !matches {
				continue
			}
			if len(pending) == 0 {
				applicable = append(applicable, action)
				continue
			}
			for _, requirement := range pending {
				existing, found := pendingByKey[requirement.key]
				if !found || compareConditionRequirements(requirement, existing) < 0 {
					pendingByKey[requirement.key] = requirement
				}
			}
		}
		if len(pendingByKey) == 0 {
			return applicable, nil
		}

		pending := make([]conditionRequirement, 0, len(pendingByKey))
		for _, requirement := range pendingByKey {
			pending = append(pending, requirement)
		}
		slices.SortFunc(pending, compareConditionRequirements)
		next := pending[0]
		truth, err := resolveCondition(ctx, resolver, next.key)
		if err != nil {
			return nil, fmt.Errorf("planning.Domain.ApplicableActions: %w", err)
		}
		resolved[next.key] = truth
	}
}

// ResolvedState overlays successful evaluator observations on state without
// mutating the immutable search snapshot. Definite values already present in
// state win, so simulated action effects cannot be overwritten by the base
// observation. Planner score functions should use this view after resolving
// applicability requirements.
func (d *Domain) ResolvedState(state core.WorldState, resolver ConditionResolver) (core.WorldState, error) {
	switch {
	case d == nil:
		return nil, errors.New("planning.Domain.ResolvedState: domain is nil")
	case nilvalue.Is(state):
		return nil, errors.New("planning.Domain.ResolvedState: world state is nil")
	case nilvalue.Is(resolver):
		return state, nil
	}

	resolved, err := snapshotResolvedConditions(resolver)
	if err != nil {
		return nil, fmt.Errorf("planning.Domain.ResolvedState: %w", err)
	}
	values := state.Conditions()
	overlay := make(core.ConditionSet)
	for _, key := range slices.Sorted(maps.Keys(resolved)) {
		truth := resolved[key]
		ref, ok := d.conditionRef(key)
		if !ok || ref.Kind != ConditionEvaluator {
			return nil, fmt.Errorf("planning.Domain.ResolvedState: condition resolver returned undeclared evaluator %q", key)
		}
		if values[key] == core.Unknown && truth != core.Unknown {
			overlay[key] = truth
		}
	}
	if len(overlay) == 0 {
		return state, nil
	}
	return state.Apply(overlay), nil
}

func (d *Domain) canonicalActionCandidates(actions []core.Action) ([]core.Action, error) {
	candidates := make([]core.Action, 0, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for index, action := range actions {
		if nilvalue.Is(action) {
			continue
		}
		metadata, err := inspectActionMetadata(action)
		if err != nil {
			return nil, fmt.Errorf("action[%d]: %w", index, err)
		}
		if _, duplicate := seen[metadata.Name]; duplicate {
			continue
		}
		canonical, ok := d.action(metadata.Name)
		if !ok {
			return nil, fmt.Errorf("action[%d] %q is outside the domain", index, metadata.Name)
		}
		seen[metadata.Name] = struct{}{}
		candidates = append(candidates, canonical)
	}
	return candidates, nil
}

// inspectRequirements classifies a conjunction without running user code.
// Known mismatches dominate unresolved evaluators: when one requirement is
// already false there is no reason to observe any other member of the same
// conjunction.
func (d *Domain) inspectRequirements(
	values core.ConditionSet,
	resolved core.ConditionSet,
	requirements core.ConditionSet,
	canResolve bool,
) ([]conditionRequirement, bool) {
	var pending []conditionRequirement
	for _, key := range slices.Sorted(maps.Keys(requirements)) {
		required := requirements[key]
		actual := values[key]
		truth, observed := resolved[key]
		if observed {
			actual = truth
		}
		if actual == required {
			continue
		}
		ref, ok := d.conditionRef(key)
		if observed || actual != core.Unknown || !canResolve || !ok || ref.Kind != ConditionEvaluator {
			return nil, false
		}
		pending = append(pending, conditionRequirement{
			key:      key,
			required: required,
			cost:     ref.Cost,
			order:    d.conditionOrder[key],
		})
	}
	return pending, true
}

func (d *Domain) conditionRef(key string) (ConditionRef, bool) {
	ref, ok := d.conditionByKey[key]
	return ref, ok
}

func compareConditionRequirements(left, right conditionRequirement) int {
	if left.cost < right.cost {
		return -1
	}
	if left.cost > right.cost {
		return 1
	}
	if left.order < right.order {
		return -1
	}
	if left.order > right.order {
		return 1
	}
	if left.key < right.key {
		return -1
	}
	if left.key > right.key {
		return 1
	}
	return 0
}

func resolveCondition(ctx context.Context, resolver ConditionResolver, key string) (truth core.Truth, err error) {
	if nilvalue.Is(resolver) {
		return core.Unknown, nil
	}
	if err := ctx.Err(); err != nil {
		return core.Unknown, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			truth = core.Unknown
			err = panicerr.New(fmt.Sprintf("condition resolver %T panicked resolving %q", resolver, key), recovered)
		}
	}()
	truth, err = resolver.Resolve(ctx, key)
	if err != nil {
		return core.Unknown, fmt.Errorf("resolve condition %q: %w", key, err)
	}
	if !truth.Valid() {
		return core.Unknown, fmt.Errorf("resolve condition %q: invalid truth value %d", key, truth)
	}
	return truth, nil
}

func snapshotResolvedConditions(resolver ConditionResolver) (conditions core.ConditionSet, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			conditions = nil
			err = panicerr.New(fmt.Sprintf("condition resolver %T ResolvedConditions panicked", resolver), recovered)
		}
	}()
	conditions = maps.Clone(resolver.ResolvedConditions())
	if err := conditions.Validate(); err != nil {
		return nil, fmt.Errorf("condition resolver %T returned invalid resolved conditions: %w", resolver, err)
	}
	return conditions, nil
}
