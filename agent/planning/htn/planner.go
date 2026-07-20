package htn

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

const defaultMaxRecursion = 64

// plannerTracer is the package-level tracer for the HTN planner. It shares the
// `lynx/agent/planner` namespace with the other planners; backends distinguish
// algorithms by span name ("htn.plan").
var plannerTracer = otel.Tracer(planning.TracerName)

// Planner is the concrete HTN planner. Library is supplied at
// construction; the planner is otherwise stateless and safe to share
// across goroutines.
type Planner struct {
	tasks        map[string]Task
	maxRecursion int
}

// NewPlanner returns an HTN planner backed by library. maxRecursion
// caps the decomposition depth to guard against cyclic task graphs.
// The planner owns an immutable library snapshot. Construction rejects nil
// libraries and unresolved subtask references.
func NewPlanner(library *Library) (*Planner, error) {
	if library == nil {
		return nil, errors.New("htn.NewPlanner: library must not be nil")
	}
	tasks := library.snapshot()
	for taskName, task := range tasks {
		for methodIndex, method := range task.Methods {
			for subtaskIndex, subtask := range method.Subtasks {
				if _, ok := tasks[subtask]; !ok {
					return nil, fmt.Errorf("htn.NewPlanner: task %q method[%d] subtask[%d] references unknown task %q", taskName, methodIndex, subtaskIndex, subtask)
				}
			}
		}
	}
	return &Planner{tasks: tasks, maxRecursion: defaultMaxRecursion}, nil
}

// Name is the planner's extension identifier — the value an agent's
// [core.AgentConfig.PlannerName] must match to select this planner.
func (p *Planner) Name() string { return planning.HTNPlannerName }

// PlanToGoal decomposes the task whose name matches goal.Name. Goals
// without a matching task return (nil, nil) so the runtime can fall
// through to a different planner if registered.
//
// Errors are reserved for *structural* problems — exceeded recursion
// depth or a method referencing an unknown subtask name. Soft
// failures (a method's preconditions don't hold; a subtask's body
// has no applicable method) cause the planner to backtrack to the
// next sibling method, returning (nil, nil) only when every option
// is exhausted.
func (p *Planner) PlanToGoal(
	ctx context.Context,
	start core.WorldState,
	domain *planning.Domain,
	goal *core.Goal,
	options planning.Options,
) (result *planning.Plan, err error) {
	if err = domain.ValidatePlanInputs(start, goal); err != nil {
		return nil, err
	}

	ctx, span := plannerTracer.Start(ctx, planning.HTNPlannerName+".plan",
		trace.WithAttributes(
			attribute.String(planning.PlannerNameKey, p.Name()),
			attribute.String(planning.GoalNameKey, goal.Name()),
		),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if result != nil {
			span.SetAttributes(attribute.Int(planning.PlanLengthKey, len(result.Actions())))
		}
		span.End()
	}()
	if goal.SatisfiedBy(start) {
		return planning.NewPlan(nil, goal), nil
	}

	root, ok := p.tasks[goal.Name()]
	if !ok {
		return nil, nil
	}
	actions, finalState, ok, err := p.decompose(ctx, root, start, options.ExcludedActions, 0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	if !goal.SatisfiedBy(finalState) {
		return nil, fmt.Errorf("htn: task %q decomposition does not satisfy its goal", root.Name)
	}
	for index, candidate := range actions {
		name := candidate.Metadata().Name
		canonical, found := domainAction(domain, name)
		if !found {
			return nil, fmt.Errorf("htn: task %q action[%d] %q is outside the planning domain", root.Name, index, name)
		}
		actions[index] = canonical
	}
	result = planning.NewPlan(actions, goal)
	return result, nil
}

func domainAction(domain *planning.Domain, name string) (core.Action, bool) {
	for _, action := range domain.Actions() {
		if action != nil && action.Metadata().Name == name {
			return action, true
		}
	}
	return nil, false
}
