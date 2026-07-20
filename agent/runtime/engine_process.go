package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// createProcess assembles a Process and its dependencies
// (blackboard, state reader, planner). The process is registered in
// the engine's map before being returned so concurrent
// Resume / Kill calls can find it.
func (e *Engine) createProcess(
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.createProcess: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.createProcess: %w", err)
	}
	return e.createProcessFromDeployment(deployment, bindings, options)
}

// createProcessFromDeployment is the exact-identity construction path used by
// advanced entry points such as child processes and agent tools. The caller
// has already resolved and ownership-checked the immutable deployment handle,
// so construction cannot drift to a newer active route.
func (e *Engine) createProcessFromDeployment(
	deployment *Deployment,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("runtime.Engine.createProcessFromDeployment: deployment is nil")
	}
	agent := deployment.agent
	processOptions, err := snapshotProcessOptions(options)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}
	dependencies, err := e.prepareProcessDependencies(options.Dependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}
	bindings = bindings.Clone()

	blackboard := e.resolveBlackboard(options.Blackboard)
	bindBlackboardSeed(blackboard, bindings)

	planner, err := e.resolvePlanner(agent, processOptions.extensions)
	if err != nil {
		return nil, err
	}

	domain := planning.DomainForAgent(agent)
	processID := e.idGenerator().Next()
	process := newProcess(processID, deployment, &processOptions, blackboard, dependencies, planner, domain, e)

	// state reader + per-process event multicast both close over the
	// assembled pointer, so they're wired after construction.
	process.wireRuntimeDeps(processOptions.extensions)

	e.processes.register(process)
	process.publishEvent(context.Background(), event.ProcessCreated{
		Header:   event.NewHeader(processID),
		Bindings: bindings,
	})
	return process, nil
}

func (e *Engine) deploymentForProcess(agent *core.Agent) (*Deployment, error) {
	if deployment, ok := e.catalog.forSource(agent); ok {
		return deployment, nil
	}
	// Standard Run/Start accept a definition for convenience, but execution
	// must never bind an uncataloged deployment: its snapshot would carry an
	// DeploymentRef that Restore cannot resolve. Deploy is idempotent for the
	// same ref and preserves the explicit conflict/replace semantics.
	return e.Deploy(agent)
}

func (e *Engine) createChild(
	deployment *Deployment,
	parent *Process,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("runtime.Engine.createChild: deployment is nil")
	}
	if parent == nil {
		return nil, errors.New("runtime.Engine.createChild: parent process is nil")
	}
	if options.Blackboard == nil {
		options.Blackboard = parent.blackboard.Clone()
	}
	// A child shares its parent's event stream: process-scope
	// EventListener extensions propagate down so the whole delegation
	// subtree surfaces on the listener the parent registered (each event
	// keeps its own ProcessID, so a consumer can tell parent from child).
	// Listeners are the only capability inherited — blackboard / planner /
	// tool extensions stay scoped to the process that declared them. No-op
	// when the parent registered no listeners, so the historical "child
	// events reach only the engine multicast" behavior is unchanged for
	// callers that don't observe per-process.
	options.Extensions = parent.childExtensions(options.Extensions)

	child, err := e.createProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		return nil, err
	}
	child.parentID = parent.id
	child.depth = parent.depth + 1
	parent.budget.addChild(child)
	return child, nil
}

// resolvePlanner finds the [planning.Planner] for agent by matching
// [core.AgentConfig.PlannerName] against registered Planner extensions:
// process-scope extensions take priority over engine-scope. An empty
// PlannerName resolves through [planning.DefaultPlannerName]. The runtime intentionally knows no
// concrete planner — the composition root registers them as extensions
// (agent.NewEngine registers goap + reactive by default).
func (e *Engine) resolvePlanner(agent *core.Agent, processExtensions []core.Extension) (planning.Planner, error) {
	name := planning.EffectivePlannerName(agent.PlannerName())

	if planner := findPlannerByName(processExtensions, name); planner != nil {
		return planner, nil
	}
	if planner := findPlannerByName(e.extensions.list, name); planner != nil {
		return planner, nil
	}

	return nil, fmt.Errorf("runtime.Engine.resolvePlanner: agent %q requests planner %q which is not registered — register a planning.Planner extension with that Name (agent.NewEngine registers goap + reactive by default)", agent.Name(), name)
}

// findPlannerByName walks extensions for a [planning.Planner] whose
// Name() matches. Returns nil when none matches.
func findPlannerByName(extensions []core.Extension, name string) planning.Planner {
	for _, extension := range extensions {
		planner, ok := extension.(planning.Planner)
		if !ok {
			continue
		}
		if planner.Name() == name {
			return planner
		}
	}
	return nil
}

// resolveBlackboard picks the [core.Blackboard] for a fresh process —
// per-call value wins; otherwise the most-recently-registered
// [core.Blackboard] extension is used as a prototype, with [Clone]
// producing the isolated per-process instance; otherwise the built-in
// in-memory implementation is constructed fresh.
func (e *Engine) resolveBlackboard(supplied core.Blackboard) core.Blackboard {
	if supplied != nil {
		return supplied
	}
	if prototype := e.blackboardPrototype(); prototype != nil {
		if blackboard := prototype.Clone(); blackboard != nil {
			return blackboard
		}
	}
	return newInMemoryBlackboard()
}

// validateProcessExtensions enforces the per-process invariants:
// nil rejected, empty Names rejected, no duplicate Names within the
// slice. Process-scope Names ARE allowed to collide with
// engine-scope Names — that's the explicit override mechanism.
func validateProcessExtensions(extensions []core.Extension) error {
	if len(extensions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(extensions))
	for index, extension := range extensions {
		if valueIsNil(extension) {
			return fmt.Errorf("ProcessOptions.Extensions[%d] is nil", index)
		}
		name := extension.Name()
		if name == "" {
			return fmt.Errorf("ProcessOptions.Extensions[%d] (%T) returned empty Name()", index, extension)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("ProcessOptions.Extensions: duplicate name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// bindBlackboardSeed applies the caller's initial bindings.
// [core.DefaultBindingName] uses Bind() so the dual-binding behavior
// kicks in; other keys go through Set so their explicit name wins.
func bindBlackboardSeed(blackboard core.Blackboard, bindings core.Bindings) {
	for key, value := range bindings.All() {
		if key == core.DefaultBindingName {
			blackboard.Bind(value)
			continue
		}
		blackboard.Store(key, value)
	}
}
