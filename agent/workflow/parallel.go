package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// ParallelConfig configures a fan-out across N sub-agents that all
// consume the same In type and produce the same Element type, then a
// single Joiner consolidates the per-agent outputs into Result.
//
// Each parallel sub-agent runs as its own child process via
// [runtime.SpawnChildFresh]; child processes get an isolated blackboard
// seeded only with the input, so peer sub-agents cannot see each other's
// intermediate writes during the parallel phase. This mirrors ADK's
// ParallelAgent branch-isolation design (avoids LLM context
// cross-pollination when each sub-agent drives its own LLM tool loop).
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
	// results is in the same order as Agents (slot indexing is
	// preserved, not completion order). Required.
	Joiner func(ctx context.Context, pc *core.ProcessContext, results []Element) (Result, error)
}

// Parallel compiles spec into a deployable agent that runs every
// sub-agent in parallel and joins their outputs.
//
// Parallel is a thin specialization of [ScatterGather]: each sub-agent
// becomes a generator that spawns one child process and extracts its
// typed Element. The fan-out machinery (errgroup, the MaxConcurrency
// cap, slot-indexed results, and the produces-Result goal) lives once
// in ScatterGather; Parallel only supplies the agent→generator adapter.
// The compiled agent therefore carries ScatterGather's two actions
// ("{Name}-scatter" / "{Name}-gather") and its single Result goal, so
// [runtime.Platform.RunAgent] terminates when the Joiner has bound the
// Result. Failure of any sub-agent (spawn error, non-Completed status,
// or missing Element) cancels the errgroup and propagates as the
// process failure, naming the offending agent.
//
// Returns an error on nil platform, missing Name, empty Agents, a nil
// sub-agent, or nil Joiner — caller decides whether to surface, retry,
// or panic.
func Parallel[In, Element, Result any](
	platform *runtime.Platform,
	spec ParallelConfig[In, Element, Result],
) (*core.Agent, error) {
	if platform == nil {
		return nil, errors.New("workflow.Parallel: platform must not be nil")
	}
	if spec.Name == "" {
		return nil, errors.New("workflow.Parallel: Name must not be empty")
	}
	if len(spec.Agents) == 0 {
		return nil, errors.New("workflow.Parallel: Agents must not be empty")
	}
	if spec.Joiner == nil {
		return nil, errors.New("workflow.Parallel: Joiner must not be nil")
	}
	for i, a := range spec.Agents {
		if a == nil {
			return nil, fmt.Errorf("workflow.Parallel: Agents[%d] is nil", i)
		}
	}

	// Build one generator per sub-agent: spawn a fresh child, surface its
	// failure, extract the typed Element. Errors name the agent so the
	// ScatterGather-level "scatter generator N" wrap stays attributable.
	generators := make([]func(context.Context, *core.ProcessContext, In) (Element, error), len(spec.Agents))
	for i, sub := range spec.Agents {
		generators[i] = func(ctx context.Context, _ *core.ProcessContext, in In) (Element, error) {
			var zero Element
			child, err := runtime.SpawnChildFresh(ctx, platform, sub, in)
			if err != nil {
				return zero, fmt.Errorf("agent %q: %w", sub.Name, err)
			}
			if err := child.TerminalError(); err != nil {
				return zero, fmt.Errorf("agent %q: %w", sub.Name, err)
			}
			out, ok := core.ResultOfType[Element](child)
			if !ok {
				return zero, fmt.Errorf("agent %q produced no %T", sub.Name, zero)
			}
			return out, nil
		}
	}

	return ScatterGather(ScatterGatherConfig[In, Element, Result]{
		Name:           spec.Name,
		Description:    spec.Description,
		MaxConcurrency: spec.MaxConcurrency,
		Generators:     generators,
		Joiner:         spec.Joiner,
	})
}
