package workflow

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/Tangerg/lynx/agent/core"
)

// scatterOutput keeps the fan-out result distinct on the workflow blackboard.
type scatterOutput[Element any] struct {
	Items []Element
}

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
	Generators []func(ctx context.Context, process *core.ProcessContext, input In) (Element, error)

	// Joiner consolidates the per-generator outputs into the final
	// Result. results is in the same order as Generators. Required.
	Joiner func(ctx context.Context, process *core.ProcessContext, results []Element) (Result, error)
}

// ScatterGather compiles config into a deployable [*core.Agent].
//
// The agent has two actions:
//
//  1. "{Name}-scatter" — runs every generator in parallel under
//     errgroup and binds its private fan-out result on the blackboard.
//  2. "{Name}-gather"  — preconditioned on the bound list; runs
//     Joiner; binds Result.
//
// The single goal targets Result, so [Engine.Run] terminates
// when Joiner has bound it.
//
// Returns an error on missing Name, empty Generators, or nil Joiner.
func ScatterGather[In, Element, Result any](config ScatterGatherConfig[In, Element, Result]) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.ScatterGather: Name must not be empty")
	}
	if len(config.Generators) == 0 {
		return nil, errors.New("workflow.ScatterGather: Generators must not be empty")
	}
	if config.Joiner == nil {
		return nil, errors.New("workflow.ScatterGather: Joiner must not be nil")
	}
	for index, generator := range config.Generators {
		if generator == nil {
			return nil, fmt.Errorf("workflow.ScatterGather: Generators[%d] must not be nil", index)
		}
	}

	name := config.Name
	description := config.Description
	maxConcurrency := config.MaxConcurrency
	generators := slices.Clone(config.Generators)
	joiner := config.Joiner

	scatter := core.NewAction[In, scatterOutput[Element]](
		name+"-scatter",
		func(ctx context.Context, process *core.ProcessContext, input In) (scatterOutput[Element], error) {
			items := make([]Element, len(generators))
			branches := make([]*core.ProcessContext, len(generators))
			for index := range branches {
				branches[index] = process.ForParallelBranch()
			}
			group, groupContext := errgroup.WithContext(ctx)
			var slots *semaphore.Weighted
			if maxConcurrency > 0 {
				slots = semaphore.NewWeighted(int64(maxConcurrency))
			}
			var schedulingErr error
			for index, generator := range generators {
				if slots != nil {
					if err := slots.Acquire(groupContext, 1); err != nil {
						schedulingErr = err
						break
					}
				}
				if err := groupContext.Err(); err != nil {
					if slots != nil {
						slots.Release(1)
					}
					schedulingErr = err
					break
				}
				group.Go(func() error {
					if slots != nil {
						defer slots.Release(1)
					}
					if err := groupContext.Err(); err != nil {
						return err
					}
					// Each generator runs in its own goroutine, so hand it a
					// sibling-safe branch created before fan-out: scratch and
					// Blackboard writes are isolated; only the returned item is
					// joined in this stable index.
					output, err := generator(groupContext, branches[index], input)
					if err != nil {
						return fmt.Errorf("scatter generator %d: %w", index, err)
					}
					items[index] = output
					return nil
				})
			}
			if err := group.Wait(); err != nil {
				return scatterOutput[Element]{}, err
			}
			if schedulingErr != nil {
				return scatterOutput[Element]{}, schedulingErr
			}
			return scatterOutput[Element]{Items: items}, nil
		},
		core.ActionConfig{
			Description: "fan-out generators in parallel",
		},
	)

	gather := core.NewAction[scatterOutput[Element], Result](
		name+"-gather",
		func(ctx context.Context, process *core.ProcessContext, input scatterOutput[Element]) (Result, error) {
			return joiner(ctx, process, input.Items)
		},
		core.ActionConfig{
			Description: "consolidate parallel results",
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        name,
		Description: description,
		Actions:     []core.Action{scatter, gather},
		Goals:       []*core.Goal{core.NewOutputGoal[Result](core.GoalConfig{Name: name, Description: "produce " + core.TypeName[Result]()})},
	}), nil
}
