package workflow

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/agent/core"
)

// ScatterGatherConfig configures a scatter-gather workflow: every
// Generator runs in parallel against the workflow input, then a
// single Joiner consolidates the per-generator outputs into the
// final Result.
//
// Type parameters:
//   - In      — the workflow's input type, fed to every generator;
//   - Element — what each generator produces;
//   - Result  — the joined output.
//
// Each generator runs in its own goroutine; Joiner sees the slice of
// Elements (in generator order) only after every generator has
// completed (or any has errored).
type ScatterGatherConfig[In, Element, Result any] struct {
	// Name names the produced agent + its goal + the action names.
	// Required.
	Name string

	// Description is the agent's human-facing summary. Optional.
	Description string

	// MaxConcurrency caps in-flight generators. <=0 means unbounded.
	MaxConcurrency int

	// Generators is the parallel fan-out. Each receives the same
	// In and produces an Element. Must be non-empty.
	Generators []func(ctx context.Context, pc *core.ProcessContext, in In) (Element, error)

	// Joiner consolidates the per-generator outputs into the final
	// Result. results is in the same order as Generators. Required.
	Joiner func(ctx context.Context, pc *core.ProcessContext, results []Element) (Result, error)
}

// ScatterGather compiles spec into a deployable [*core.Agent].
//
// The agent has two actions:
//
//  1. "{Name}-scatter" — runs every generator in parallel under
//     errgroup, binds [ResultList][Element] on the blackboard.
//  2. "{Name}-gather"  — preconditioned on the bound list; runs
//     Joiner; binds Result.
//
// The single goal targets Result, so [Platform.RunAgent] terminates
// when Joiner has bound it.
//
// Returns an error on missing Name, empty Generators, or nil Joiner.
func ScatterGather[In, Element, Result any](spec ScatterGatherConfig[In, Element, Result]) (*core.Agent, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("workflow.ScatterGather: Name must not be empty")
	}
	if len(spec.Generators) == 0 {
		return nil, fmt.Errorf("workflow.ScatterGather: Generators must not be empty")
	}
	if spec.Joiner == nil {
		return nil, fmt.Errorf("workflow.ScatterGather: Joiner must not be nil")
	}

	scatter := core.NewAction[In, ResultList[Element]](
		spec.Name+"-scatter",
		func(ctx context.Context, pc *core.ProcessContext, in In) (ResultList[Element], error) {
			items := make([]Element, len(spec.Generators))
			g, gctx := errgroup.WithContext(ctx)
			if spec.MaxConcurrency > 0 {
				g.SetLimit(spec.MaxConcurrency)
			}
			for i, gen := range spec.Generators {
				g.Go(func() error {
					out, err := gen(gctx, pc, in)
					if err != nil {
						return fmt.Errorf("scatter generator %d: %w", i, err)
					}
					items[i] = out
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				return ResultList[Element]{}, err
			}
			return ResultList[Element]{Items: items}, nil
		},
		core.ActionConfig{
			Description: "fan-out generators in parallel",
			QoS:         singleAttempt,
		},
	)

	gather := core.NewAction[ResultList[Element], Result](
		spec.Name+"-gather",
		func(ctx context.Context, pc *core.ProcessContext, in ResultList[Element]) (Result, error) {
			return spec.Joiner(ctx, pc, in.Items)
		},
		core.ActionConfig{
			Description: "consolidate parallel results",
			QoS:         singleAttempt,
		},
	)

	return core.NewAgent(&core.AgentConfig{
		Name:        spec.Name,
		Description: spec.Description,
		Actions:     []core.Action{scatter, gather},
		Goals: []*core.Goal{core.GoalProducing[Result](core.Goal{
			Name:        spec.Name,
			Description: "produce " + core.TypeName[Result](),
		})},
	}), nil
}
