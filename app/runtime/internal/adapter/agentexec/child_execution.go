package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/chatclient"
)

var childExecutionKey = core.MustDependencyKey[childExecution]("lyra.child_execution")

type childExecution struct {
	Provider   string
	Budget     accounting.Budget
	StopReason StopReason
}

// childExecutionPolicy is rebound on every root start/restore. The first child
// invocation captures the durable turnInput from the root blackboard; returned
// child options carry a policy bound to that same root, so deeper descendants
// share one provider identity and one remaining application budget.
type childExecutionPolicy struct {
	dependencies    *core.Dependencies
	client          *chatclient.Client
	observer        toolObserver
	toolResultStore toolResultOffloader
	evictThreshold  int
	root            core.ProcessView
	provider        string
	budget          accounting.Budget
}

func childOptions(dependencies *core.Dependencies, client *chatclient.Client, observer toolObserver, toolResultStore toolResultOffloader, evictThreshold int) core.ChildOptionsFunc {
	return (childExecutionPolicy{
		dependencies:    dependencies,
		client:          client,
		observer:        observer,
		toolResultStore: toolResultStore,
		evictThreshold:  evictThreshold,
	}).options
}

func (p childExecutionPolicy) options(_ context.Context, parent core.ProcessView, _ *core.Agent) (core.ProcessOptions, error) {
	if p.dependencies == nil {
		return core.ProcessOptions{}, errors.New("agentexec: child execution requires engine dependencies")
	}
	if p.root == nil {
		input, ok := core.Get[turnInput](parent.Blackboard(), core.DefaultBindingName)
		if !ok {
			return core.ProcessOptions{}, errors.New("agentexec: child execution root has no turn input")
		}
		p.root = parent
		p.provider = input.Provider
		p.budget = accounting.Budget{
			MaxTokens:  input.MaxBudget,
			MaxCostUSD: input.MaxCostUSD,
			MaxSteps:   input.MaxSteps,
		}
	}

	dependencies := p.dependencies.Child()
	if err := core.RegisterDependency(dependencies, childExecutionKey, p.remaining()); err != nil {
		return core.ProcessOptions{}, fmt.Errorf("agentexec: register child execution context: %w", err)
	}
	var observation *toolObservation
	if p.observer != nil {
		observation = newToolObservation(p.observer, p.toolResultStore, p.evictThreshold)
		if err := core.RegisterDependency(dependencies, toolObservationKey, observation); err != nil {
			return core.ProcessOptions{}, fmt.Errorf("agentexec: register child tool observation: %w", err)
		}
	}
	options := core.ProcessOptions{
		Dependencies: dependencies,
		ChildOptions: p.options,
	}
	if p.observer != nil {
		options.Extensions = append(options.Extensions, &toolObserverMiddleware{observation: observation})
	}
	if p.client != nil {
		options.Extensions = append(options.Extensions, perRunChatClient{client: p.client})
	}
	return options, nil
}

func (p childExecutionPolicy) remaining() childExecution {
	execution := childExecution{Provider: p.provider}
	if p.root == nil {
		return execution
	}
	cost, tokens, _ := p.root.Usage()

	if p.budget.MaxTokens > 0 {
		remaining := p.budget.MaxTokens - int64(tokens)
		if remaining <= 0 {
			execution.StopReason = StopReasonBudget
		} else {
			execution.Budget.MaxTokens = remaining
		}
	}
	if p.budget.MaxCostUSD > 0 {
		remaining := p.budget.MaxCostUSD - cost
		if remaining <= 0 {
			execution.StopReason = StopReasonBudget
		} else {
			execution.Budget.MaxCostUSD = remaining
		}
	}
	if p.budget.MaxSteps > 0 {
		remaining := p.budget.MaxSteps - len(p.root.ModelCalls())
		if remaining <= 0 {
			if execution.StopReason == StopReasonNone {
				execution.StopReason = StopReasonSteps
			}
		} else {
			execution.Budget.MaxSteps = remaining
		}
	}
	return execution
}

func childExecutionFrom(dependencies *core.Dependencies) (childExecution, error) {
	execution, err := core.LookupDependency(dependencies, childExecutionKey)
	if errors.Is(err, core.ErrDependencyNotFound) {
		return childExecution{}, nil
	}
	if err != nil {
		return childExecution{}, fmt.Errorf("agentexec: resolve child execution context: %w", err)
	}
	return execution, nil
}
