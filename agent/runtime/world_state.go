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

// worldStateReader projects blackboard contents into planner state. It reads
// Blackboard-backed sources immediately and leaves named evaluators Unknown
// until a planner requests them through Resolve.
type worldStateReader struct {
	domain     *planning.Domain
	blackboard core.Blackboard
	process    *Process

	// namedConditions indexes domain.Conditions by Name() so on-demand
	// resolution is a map lookup rather than a linear scan.
	namedConditions map[string]core.Condition

	resolutionMu sync.Mutex
	resolutions  map[string]conditionResolution
}

type conditionResolution struct {
	truth core.Truth
	err   error
}

func newWorldStateReader(domain *planning.Domain, blackboard core.Blackboard, process *Process) *worldStateReader {
	namedConditions := make(map[string]core.Condition, len(domain.Conditions()))
	for _, condition := range domain.Conditions() {
		if nilvalue.Is(condition) {
			continue
		}
		namedConditions[condition.Name()] = condition
	}
	return &worldStateReader{
		domain:          domain,
		blackboard:      blackboard,
		process:         process,
		namedConditions: namedConditions,
		resolutions:     make(map[string]conditionResolution),
	}
}

func (r *worldStateReader) read(ctx context.Context) (core.WorldState, error) {
	r.resolutionMu.Lock()
	r.resolutions = make(map[string]conditionResolution)
	r.resolutionMu.Unlock()

	state := core.ConditionSet{}
	env := &core.ConditionEnv{Process: r.process, Blackboard: r.blackboard}

	for condition := range r.domain.KnownConditions() {
		if condition.Kind == planning.ConditionEvaluator {
			state[condition.Key] = core.Unknown
			continue
		}
		truth, err := r.evaluateCondition(ctx, condition, env)
		if err != nil {
			return nil, err
		}
		state[condition.Key] = truth
	}
	return planning.NewState(state), nil
}

// Resolve evaluates one named condition at most once per world-state read.
// Planners may encounter the same predicate in multiple goals, actions, or
// simulated states; all of them observe one stable answer for the tick.
func (r *worldStateReader) Resolve(ctx context.Context, name string) (core.Truth, error) {
	r.resolutionMu.Lock()
	defer r.resolutionMu.Unlock()
	if result, ok := r.resolutions[name]; ok {
		return result.truth, result.err
	}

	ref, ok := r.domain.ConditionRef(name)
	if !ok || ref.Kind != planning.ConditionEvaluator {
		return core.Unknown, fmt.Errorf("runtime: condition %q is not evaluator-backed", name)
	}
	env := &core.ConditionEnv{Process: r.process, Blackboard: r.blackboard}
	truth, err := r.evaluateCondition(ctx, ref, env)
	r.resolutions[name] = conditionResolution{truth: truth, err: err}
	return truth, err
}

// ResolvedConditions returns an ownership-isolated view of the successful
// named-condition observations made since the latest read.
func (r *worldStateReader) ResolvedConditions() core.ConditionSet {
	r.resolutionMu.Lock()
	defer r.resolutionMu.Unlock()
	resolved := make(core.ConditionSet, len(r.resolutions))
	for name, result := range r.resolutions {
		if result.err == nil {
			resolved[name] = result.truth
		}
	}
	return resolved
}

// User-supplied Conditions run inside [safeEvaluateCondition] so a panicking
// implementation fails the process through the ordinary observation error
// path rather than tearing down the host.
func (r *worldStateReader) evaluateCondition(ctx context.Context, ref planning.ConditionRef, env *core.ConditionEnv) (core.Truth, error) {
	switch ref.Kind {
	case planning.ConditionBinding:
		return core.TruthOf(r.blackboard.HasValue(ref.Binding.Name, ref.Binding.Type)), nil
	case planning.ConditionEvaluator:
		condition, ok := r.namedConditions[ref.Key]
		if !ok {
			return core.Unknown, fmt.Errorf("runtime: condition %q has no evaluator", ref.Key)
		}
		conditionEnv := *env
		conditionEnv.RunInteraction = func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return r.process.runInteraction(ctx, core.ConditionInteractionID(ref.Key), input)
		}
		truth, err := safeEvaluateCondition(ctx, condition, &conditionEnv)
		if err != nil {
			return core.Unknown, fmt.Errorf("runtime: condition %q: %w", ref.Key, err)
		}
		if !truth.Valid() {
			return core.Unknown, fmt.Errorf("runtime: condition %q returned invalid truth value %d", ref.Key, truth)
		}
		return truth, nil
	case planning.ConditionActionRun:
		value, _ := r.blackboard.Condition(ref.Key)
		return core.TruthOf(value), nil
	case planning.ConditionFact:
		value, ok := r.blackboard.Condition(ref.Key)
		if !ok {
			return core.Unknown, nil
		}
		return core.TruthOf(value), nil
	default:
		return core.Unknown, fmt.Errorf("runtime: condition %q has invalid source kind %d", ref.Key, ref.Kind)
	}
}

// safeEvaluateCondition contains evaluator panics at the user-code boundary.
// Unknown remains a valid explicit result; a panic is an execution failure and
// must not be disguised as domain uncertainty.
func safeEvaluateCondition(ctx context.Context, condition core.Condition, env *core.ConditionEnv) (result core.Truth, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("condition evaluator panicked", recovered)
		}
	}()
	return condition.Evaluate(ctx, env), nil
}
