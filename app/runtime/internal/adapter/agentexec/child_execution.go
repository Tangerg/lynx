package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/chatclient"
)

// childExecutionPolicy propagates application-owned provider, dependencies,
// accounting projection, and UI hooks. Agent supplies the tree-wide budget
// authority automatically; children never receive a second limit calculation.
type childExecutionPolicy struct {
	engine          *Engine
	dependencies    *core.Dependencies
	client          *chatclient.Client
	provider        string
	observer        toolObserver
	toolResultStore toolResultOffloader
	evictThreshold  int
	chatMiddleware  *core.ChatMiddleware
	usage           *usageLedger
}

func childOptions(
	engine *Engine,
	dependencies *core.Dependencies,
	client *chatclient.Client,
	provider string,
	observer toolObserver,
	toolResultStore toolResultOffloader,
	evictThreshold int,
	chatMiddleware *core.ChatMiddleware,
	usage *usageLedger,
) core.ChildOptionsFunc {
	return (childExecutionPolicy{
		engine:          engine,
		dependencies:    dependencies,
		client:          client,
		provider:        provider,
		observer:        observer,
		toolResultStore: toolResultStore,
		evictThreshold:  evictThreshold,
		chatMiddleware:  chatMiddleware,
		usage:           usage,
	}).options
}

func (p childExecutionPolicy) options(_ context.Context, _ core.ProcessView, _ core.AgentDescriptor) (core.ProcessOptions, error) {
	if p.dependencies == nil {
		return core.ProcessOptions{}, errors.New("agentexec: child execution requires engine dependencies")
	}
	dependencies := p.dependencies.Child()
	if p.usage == nil {
		return core.ProcessOptions{}, errors.New("agentexec: child execution requires usage ledger")
	}
	if err := core.RegisterDependency(dependencies, usageLedgerKey, p.usage); err != nil {
		return core.ProcessOptions{}, fmt.Errorf("agentexec: register child usage ledger: %w", err)
	}
	var observation *toolObservation
	if p.observer != nil {
		observation = newToolObservation(p.observer, p.toolResultStore, p.evictThreshold)
		if err := core.RegisterDependency(dependencies, toolObservationKey, observation); err != nil {
			return core.ProcessOptions{}, fmt.Errorf("agentexec: register child tool observation: %w", err)
		}
	}
	options := core.ProcessOptions{
		Dependencies:   dependencies,
		ChildOptions:   p.options,
		ChatMiddleware: p.chatMiddleware,
	}
	if p.observer != nil {
		options.Extensions = append(options.Extensions, &toolObserverMiddleware{observation: observation})
	}
	options.Extensions = append(options.Extensions, &interactionProjection{
		engine:      p.engine,
		provider:    p.provider,
		usage:       p.usage,
		observation: observation,
	})
	if p.client != nil {
		options.Extensions = append(options.Extensions, perRunChatClient{client: p.client})
	}
	return options, nil
}
