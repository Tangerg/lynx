package workflow

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// ParallelConfig configures a fan-out across N sub-agents that all
// consume the same In type and produce the same Element type, then a
// single Joiner consolidates the per-agent outputs into Result.
//
// Each parallel sub-agent runs as its own child process via
// [runtime.SpawnChild]; child processes inherit the parent's blackboard
// via [core.Blackboard.Spawn], giving every branch isolated state — peer
// sub-agents cannot see each other's intermediate writes during the
// parallel phase. This mirrors ADK's ParallelAgent branch-isolation
// design (avoids LLM context cross-pollination when each sub-agent
// drives its own LLM tool loop).
type ParallelConfig[In, Element, Result any] struct {
	// Name names the produced agent + its goal + the action names. Required.
	Name string

	// Description is the agent's human-facing summary.
	Description string

	// MaxConcurrency caps in-flight sub-agents. <=0 means unbounded.
	MaxConcurrency int

	// Agents is the parallel set. Each must accept In and produce
	// Element via its own goal/action declarations. Must be non-empty.
	Agents []*core.Agent

	// Joiner consolidates the per-agent outputs into the final Result.
	// results is in the same order as Agents (errgroup preserves slot
	// indexing, not completion order). Required.
	Joiner func(ctx context.Context, pc *core.ProcessContext, results []Element) (Result, error)
}

// Parallel compiles spec into a deployable agent. The compiled
// agent has two actions:
//
//  1. "{Name}-fanout" — runs every sub-agent in parallel under errgroup,
//     binds [ResultList][Element] on the blackboard.
//  2. "{Name}-join"   — preconditioned on the bound list; runs Joiner;
//     binds Result.
//
// The single goal targets Result, so [Platform.RunAgent] terminates
// when Joiner has bound it. Failure of any sub-agent (non-Completed
// status) cancels the errgroup; the first failure is returned with the
// failing agent's index for attribution.
//
// Returns an error on missing Name, empty Agents, or nil Joiner —
// caller decides whether to surface, retry, or panic.
func Parallel[In, Element, Result any](
	platform *runtime.Platform,
	spec ParallelConfig[In, Element, Result],
) (*core.Agent, error) {
	if platform == nil {
		return nil, fmt.Errorf("workflow.Parallel: platform must not be nil")
	}
	if spec.Name == "" {
		return nil, fmt.Errorf("workflow.Parallel: Name must not be empty")
	}
	if len(spec.Agents) == 0 {
		return nil, fmt.Errorf("workflow.Parallel: Agents must not be empty")
	}
	if spec.Joiner == nil {
		return nil, fmt.Errorf("workflow.Parallel: Joiner must not be nil")
	}
	for i, a := range spec.Agents {
		if a == nil {
			return nil, fmt.Errorf("workflow.Parallel: Agents[%d] is nil", i)
		}
	}

	fanout := core.NewAction[In, ResultList[Element]](
		spec.Name+"-fanout",
		func(ctx context.Context, _ *core.ProcessContext, in In) (ResultList[Element], error) {
			results := make([]Element, len(spec.Agents))
			g, gctx := errgroup.WithContext(ctx)
			if spec.MaxConcurrency > 0 {
				g.SetLimit(spec.MaxConcurrency)
			}
			for i, sub := range spec.Agents {
				g.Go(func() error {
					child, err := runtime.SpawnChildFresh(gctx, platform, sub, in)
					if err != nil {
						return fmt.Errorf("agent %d (%s): %w", i, sub.Name, err)
					}
					if err := runtime.ChildError(child); err != nil {
						return fmt.Errorf("agent %d (%s): %w", i, sub.Name, err)
					}
					out, ok := core.ResultOfType[Element](child)
					if !ok {
						var zero Element
						return fmt.Errorf("agent %d (%s) produced no %T", i, sub.Name, zero)
					}
					results[i] = out
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				return ResultList[Element]{}, err
			}
			return ResultList[Element]{Items: results}, nil
		},
		core.ActionConfig{
			Description: "fan-out sub-agents in parallel",
			QoS:         singleAttempt,
		},
	)

	join := core.NewAction[ResultList[Element], Result](
		spec.Name+"-join",
		func(ctx context.Context, pc *core.ProcessContext, items ResultList[Element]) (Result, error) {
			return spec.Joiner(ctx, pc, items.Items)
		},
		core.ActionConfig{
			Description: "consolidate parallel sub-agent outputs",
			QoS:         singleAttempt,
		},
	)

	return core.NewAgent(core.AgentConfig{
		Name:        spec.Name,
		Description: spec.Description,
		Actions:     []core.Action{fanout, join},
		Goals: []*core.Goal{core.GoalProducing[Result](core.Goal{
			Name:        spec.Name,
			Description: "produce " + core.TypeName[Result](),
		})},
	}), nil
}
