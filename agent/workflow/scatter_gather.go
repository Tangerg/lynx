package workflow

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
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
// Each generator runs in its own goroutine without access to the parent
// ProcessContext. Joiner sees the slice of Elements in generator order only
// after every started generator has returned. A generator error cancels the
// shared context, but cancellation remains cooperative. If multiple generators
// fail, the lowest-index non-cancellation failure wins so completion timing
// cannot change the process error.
type ScatterGatherConfig[In, Element, Result any] struct {
	// Name names the produced agent + its goal + the action names.
	// Required.
	Name string

	// Description is the agent's human-facing summary. Optional.
	Description string

	// MaxConcurrency caps in-flight generators. Zero runs all declared
	// generators concurrently; negative values are invalid.
	MaxConcurrency int

	// Generators is the parallel fan-out. Each receives the same
	// In and produces an Element. Must be non-empty.
	Generators []Generator[In, Element]

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
// The single goal targets Result, so [runtime.Engine.Run] terminates
// when Joiner has bound it.
//
// Returns an error on missing Name, negative MaxConcurrency, empty Generators,
// a nil Generator, or nil Joiner.
func ScatterGather[In, Element, Result any](config ScatterGatherConfig[In, Element, Result]) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.ScatterGather: Name must not be empty")
	}
	if config.MaxConcurrency < 0 {
		return nil, errors.New("workflow.ScatterGather: MaxConcurrency must not be negative")
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
		func(ctx context.Context, _ *core.ProcessContext, input In) (scatterOutput[Element], error) {
			items := make([]Element, len(generators))
			generatorErrors := make([]error, len(generators))
			group, groupContext := errgroup.WithContext(ctx)
			if maxConcurrency > 0 {
				group.SetLimit(maxConcurrency)
			}
			var schedulingErr error
			for index, generator := range generators {
				if err := groupContext.Err(); err != nil {
					schedulingErr = err
					break
				}
				// Go blocks here once the limit is reached, which is the whole
				// admission control: a branch starts only when a slot frees.
				group.Go(func() error {
					if err := groupContext.Err(); err != nil {
						generatorErrors[index] = fmt.Errorf("workflow.ScatterGather: generator %d: %w", index, err)
						return generatorErrors[index]
					}
					output, err := invokeGenerator(groupContext, index, generator, input)
					generatorErrors[index] = err
					if err != nil {
						return err
					}
					items[index] = output
					return nil
				})
			}
			waitErr := group.Wait()
			if err := preferredGeneratorError(generatorErrors); err != nil {
				return scatterOutput[Element]{}, err
			}
			if schedulingErr != nil {
				return scatterOutput[Element]{}, schedulingErr
			}
			if waitErr != nil {
				return scatterOutput[Element]{}, waitErr
			}
			return scatterOutput[Element]{Items: items}, nil
		},
		core.ActionConfig{},
	)

	gather := core.NewAction[scatterOutput[Element], Result](
		name+"-gather",
		func(ctx context.Context, process *core.ProcessContext, input scatterOutput[Element]) (Result, error) {
			return joiner(ctx, process, input.Items)
		},
		core.ActionConfig{},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        name,
		Description: description,
		Actions:     []core.Action{scatter, gather},
		Goals:       []*core.Goal{core.NewOutputGoal[Result](core.GoalConfig{Name: name})},
	}), nil
}

func invokeGenerator[In, Out any](
	ctx context.Context,
	index int,
	generator Generator[In, Out],
	input In,
) (output Out, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var zero Out
			output = zero
			err = panicerr.New(fmt.Sprintf("workflow.ScatterGather: generator %d panicked", index), recovered)
		}
	}()
	output, err = generator(ctx, input)
	if err != nil {
		return output, fmt.Errorf("workflow.ScatterGather: generator %d: %w", index, err)
	}
	return output, nil
}

func preferredGeneratorError(generatorErrors []error) error {
	var cancellation error
	for _, err := range generatorErrors {
		if err == nil {
			continue
		}
		if cancellation == nil {
			cancellation = err
		}
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
	return cancellation
}
