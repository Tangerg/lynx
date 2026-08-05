package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/agent/planning"
)

// worldStateObserver projects blackboard contents into planner state. It reads
// Blackboard-backed sources immediately and leaves named evaluators Unknown
// until a planner requests them through Resolve.
type worldStateObserver struct {
	domain     *planning.Domain
	blackboard core.Blackboard
	process    *Process

	// evaluators indexes domain.Conditions by Name() so on-demand
	// resolution is a map lookup rather than a linear scan.
	evaluators map[string]core.Condition

	resolutionMu sync.Mutex
	resolutions  map[string]conditionResolution
}

type conditionResolution struct {
	truth core.Truth
	err   error
}

func newWorldStateObserver(domain *planning.Domain, blackboard core.Blackboard, process *Process) *worldStateObserver {
	evaluators := make(map[string]core.Condition, len(domain.Conditions()))
	for _, condition := range domain.Conditions() {
		if nilvalue.Is(condition) {
			continue
		}
		evaluators[condition.Name()] = condition
	}
	return &worldStateObserver{
		domain:      domain,
		blackboard:  blackboard,
		process:     process,
		evaluators:  evaluators,
		resolutions: make(map[string]conditionResolution),
	}
}

func (o *worldStateObserver) observe(ctx context.Context) (core.WorldState, error) {
	o.resolutionMu.Lock()
	o.resolutions = make(map[string]conditionResolution)
	o.resolutionMu.Unlock()

	state := core.ConditionSet{}
	env := &core.ConditionEnv{Process: o.process, Blackboard: o.blackboard}

	for condition := range o.domain.ConditionRefs() {
		if condition.Source == planning.ConditionEvaluator {
			state[condition.Key] = core.Unknown
			continue
		}
		truth, err := o.observeCondition(ctx, condition, env)
		if err != nil {
			return nil, err
		}
		state[condition.Key] = truth
	}
	return planning.NewState(state), nil
}

// Resolve evaluates one named condition at most once per world-state observation.
// Planners may encounter the same predicate in multiple goals, actions, or
// simulated states; all of them observe one stable answer for the tick.
func (o *worldStateObserver) Resolve(ctx context.Context, key string) (core.Truth, error) {
	o.resolutionMu.Lock()
	defer o.resolutionMu.Unlock()
	if result, ok := o.resolutions[key]; ok {
		return result.truth, result.err
	}

	ref, ok := o.domain.ConditionRef(key)
	if !ok || ref.Source != planning.ConditionEvaluator {
		return core.Unknown, fmt.Errorf("runtime: condition %q is not evaluator-backed", key)
	}
	env := &core.ConditionEnv{Process: o.process, Blackboard: o.blackboard}
	truth, err := o.observeCondition(ctx, ref, env)
	o.resolutions[key] = conditionResolution{truth: truth, err: err}
	return truth, err
}

// ResolvedConditions returns an ownership-isolated view of the successful
// named-condition observations made since the latest observation.
func (o *worldStateObserver) ResolvedConditions() core.ConditionSet {
	o.resolutionMu.Lock()
	defer o.resolutionMu.Unlock()
	resolved := make(core.ConditionSet, len(o.resolutions))
	for name, result := range o.resolutions {
		if result.err == nil {
			resolved[name] = result.truth
		}
	}
	return resolved
}

// User-supplied Conditions run inside [callConditionEvaluator] so a panicking
// implementation fails the process through the ordinary observation error
// path rather than tearing down the host.
func (o *worldStateObserver) observeCondition(ctx context.Context, ref planning.ConditionRef, env *core.ConditionEnv) (core.Truth, error) {
	switch ref.Source {
	case planning.ConditionBinding:
		return core.TruthOf(o.blackboard.HasValue(ref.Binding.Name, ref.Binding.Type)), nil
	case planning.ConditionEvaluator:
		condition, ok := o.evaluators[ref.Key]
		if !ok {
			return core.Unknown, fmt.Errorf("runtime: condition %q has no evaluator", ref.Key)
		}
		conditionEnv := *env
		conditionEnv.RunInteraction = func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return o.process.runInteraction(ctx, core.ConditionInteractionID(ref.Key), input)
		}
		truth, err := callConditionEvaluator(ctx, condition, &conditionEnv)
		if err != nil {
			return core.Unknown, fmt.Errorf("runtime: condition %q: %w", ref.Key, err)
		}
		if !truth.Valid() {
			return core.Unknown, fmt.Errorf("runtime: condition %q returned invalid truth value %d", ref.Key, truth)
		}
		return truth, nil
	case planning.ConditionActionSuccess:
		value, _ := o.blackboard.Condition(ref.Key)
		return core.TruthOf(value), nil
	case planning.ConditionFact:
		value, ok := o.blackboard.Condition(ref.Key)
		if !ok {
			return core.Unknown, nil
		}
		return core.TruthOf(value), nil
	default:
		return core.Unknown, fmt.Errorf("runtime: condition %q has invalid source kind %d", ref.Key, ref.Source)
	}
}

// callConditionEvaluator contains evaluator panics at the user-code boundary.
// Unknown remains a valid explicit result; a panic is an execution failure and
// must not be disguised as domain uncertainty.
func callConditionEvaluator(ctx context.Context, condition core.Condition, env *core.ConditionEnv) (result core.Truth, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("condition evaluator panicked", recovered)
		}
	}()
	return condition.Evaluate(ctx, env), nil
}
