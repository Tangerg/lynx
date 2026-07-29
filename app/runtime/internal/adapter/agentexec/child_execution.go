package agentexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/chatclient"
)

// ChildProcess is the immutable executor identity available at the child
// admission boundary. It is deliberately narrower than core.ProcessView: the
// application needs lineage and creation time, not live execution state.
type ChildProcess struct {
	ProcessRef
	StartedAt time.Time
}

// AdmitChildFunc durably admits child before Agent Runtime publishes or
// executes it. Returning an error rejects the unpublished child.
type AdmitChildFunc func(ctx context.Context, child ChildProcess) error

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
	admitChild      AdmitChildFunc
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
	admitChild AdmitChildFunc,
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
		admitChild:      admitChild,
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
	if p.admitChild != nil {
		options.Extensions = append(options.Extensions, childRunAdmitter{admit: p.admitChild})
	}
	if p.client != nil {
		options.Extensions = append(options.Extensions, perRunChatClient{client: p.client})
	}
	return options, nil
}

// childRunAdmitter translates Agent Runtime's process view into the adapter's
// stable value contract. Direct Engine.RunChild calls have no SpawnCallID and
// remain SDK-internal children; only AgentTool delegation has the complete
// causal edge required for a first-class application Run.
type childRunAdmitter struct {
	admit AdmitChildFunc
}

func (childRunAdmitter) Name() string { return "lyra:child-run-admission" }

func (admitter childRunAdmitter) AdmitChild(ctx context.Context, process core.ProcessView) error {
	ref := processRef(process)
	if ref.SpawnCallID == "" {
		return nil
	}
	return admitter.admit(ctx, ChildProcess{
		ProcessRef: ref,
		StartedAt:  process.StartedAt(),
	})
}

var _ agentruntime.ChildAdmitter = childRunAdmitter{}
