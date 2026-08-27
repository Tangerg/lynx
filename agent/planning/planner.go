package planning

import "context"

// Planner finds an ordered Action sequence for one immutable Problem. It must
// be deterministic and side-effect-free for the same Problem, safe for
// concurrent calls, and honor context cancellation. found=false with nil error
// means the search proved no plan within its algorithm's complete search space;
// resource exhaustion must be returned as an error instead.
type Planner interface {
	// Plan searches one immutable Problem without mutating it or performing I/O.
	// found=false with nil error is reserved for an exhausted complete search;
	// cancellation, resource limits, invalid costs, and internal failure return
	// errors. Equivalent Problems must produce an equivalent ordered Plan.
	Plan(ctx context.Context, problem Problem) (plan Plan, found bool, err error)
}

type PlannerFunc func(ctx context.Context, problem Problem) (plan Plan, found bool, err error)

func (p PlannerFunc) Plan(
	ctx context.Context,
	problem Problem,
) (Plan, bool, error) {
	return p(ctx, problem)
}
