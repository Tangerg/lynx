package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/agent/planning"
)

// ErrProcessIdentity reports an ID generator result that cannot uniquely
// identify a new process.
var ErrProcessIdentity = errors.New("runtime: invalid process identity")

// createProcess assembles a Process and its dependencies
// (blackboard, state reader, planner). The process is registered in
// the engine's map before being returned so concurrent
// Resume / Kill calls can find it.
func (e *Engine) createProcess(
	ctx context.Context,
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.createProcess: agent definition is nil")
	}
	deployment, err := e.deploymentForProcess(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.createProcess: %w", err)
	}
	return e.createProcessFromDeployment(ctx, deployment, bindings, options)
}

// createProcessFromDeployment is the exact-identity construction path used by
// advanced entry points such as child processes and agent tools. The caller
// has already resolved and ownership-checked the immutable deployment handle,
// so construction cannot drift to a newer active route.
func (e *Engine) createProcessFromDeployment(
	ctx context.Context,
	deployment *Deployment,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, error) {
	process, eventBindings, err := e.registerProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		return nil, err
	}
	process.publishCreated(ctx, eventBindings)
	return process, nil
}

func (e *Engine) registerProcessFromDeployment(
	deployment *Deployment,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, core.Bindings, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, core.Bindings{}, errors.New("runtime.Engine.createProcessFromDeployment: deployment is nil")
	}
	agent := deployment.agent
	processOptions, err := snapshotProcessOptions(options)
	if err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}
	dependencies, err := e.prepareProcessDependencies(options.Dependencies)
	if err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}
	bindings = bindings.Clone()

	blackboard, err := e.resolveBlackboard(options.Blackboard)
	if err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}
	if err := bindBlackboardSeed(blackboard, bindings); err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}

	planner, err := e.resolvePlanner(agent, processOptions.extensions)
	if err != nil {
		return nil, core.Bindings{}, err
	}

	domain, err := planning.DomainForAgent(agent)
	if err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: domain: %w", err)
	}
	processID, err := nextProcessID(e.idGenerator())
	if err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w", err)
	}
	process := newProcess(processID, deployment, &processOptions, blackboard, dependencies, planner, domain, e)

	// state reader + per-process event multicast both close over the
	// assembled pointer, so they're wired after construction.
	process.wireRuntimeDeps(processOptions.extensions)

	if !e.processes.insert(process) {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w: duplicate ID %q", ErrProcessIdentity, processID)
	}
	return process, bindings, nil
}

func (e *Engine) deploymentForProcess(ctx context.Context, agent *core.Agent) (*Deployment, error) {
	if deployment, ok := e.catalog.forSource(agent); ok {
		return deployment, nil
	}
	// Standard Run/Start accept a definition for convenience, but execution
	// must never bind an uncataloged deployment: its snapshot would carry an
	// DeploymentRef that Restore cannot resolve. Deploy is idempotent for the
	// same ref and preserves the explicit conflict/replace semantics.
	return e.DeployContext(ctx, agent)
}

func (e *Engine) createChild(
	deployment *Deployment,
	parent *Process,
	bindings core.Bindings,
	options core.ProcessOptions,
) (*Process, core.Bindings, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, core.Bindings{}, errors.New("runtime.Engine.createChild: deployment is nil")
	}
	if parent == nil {
		return nil, core.Bindings{}, errors.New("runtime.Engine.createChild: parent process is nil")
	}
	if options.Blackboard == nil {
		return nil, core.Bindings{}, errors.New("runtime.Engine.createChild: child blackboard is nil")
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
	extensions, err := parent.childExtensions(options.Extensions)
	if err != nil {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createChild: extensions: %w", err)
	}
	options.Extensions = extensions

	child, eventBindings, err := e.registerProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		return nil, core.Bindings{}, err
	}
	child.parentID = parent.id
	child.depth = parent.depth + 1
	parent.budget.addChild(child)
	return child, eventBindings, nil
}

// resolvePlanner finds the [planning.Planner] for agent by matching
// [core.AgentConfig.PlannerName] against registered Planner extensions:
// process-scope extensions take priority over engine-scope. An empty
// PlannerName resolves through [planning.DefaultPlannerName]. The runtime intentionally knows no
// concrete planner — the composition root registers them as extensions
// (agent.NewEngine registers goap + reactive by default).
func (e *Engine) resolvePlanner(agent *core.Agent, processExtensions []core.Extension) (planning.Planner, error) {
	name := planning.EffectivePlannerName(agent.PlannerName())

	if planner, err := findPlannerByName(processExtensions, name); err != nil {
		return nil, fmt.Errorf("runtime.Engine.resolvePlanner: process extensions: %w", err)
	} else if planner != nil {
		return planner, nil
	}
	if planner, err := findPlannerByName(e.extensions.list, name); err != nil {
		return nil, fmt.Errorf("runtime.Engine.resolvePlanner: engine extensions: %w", err)
	} else if planner != nil {
		return planner, nil
	}

	return nil, fmt.Errorf("runtime.Engine.resolvePlanner: agent %q requests planner %q which is not registered — register a planning.Planner extension with that Name (agent.NewEngine registers goap + reactive by default)", agent.Name(), name)
}

// findPlannerByName walks extensions for a [planning.Planner] whose
// Name() matches. Returns nil when none matches.
func findPlannerByName(extensions []core.Extension, name string) (planning.Planner, error) {
	for _, extension := range extensions {
		planner, ok := extension.(planning.Planner)
		if !ok {
			continue
		}
		extensionName, err := extensionName(planner)
		if err != nil {
			return nil, err
		}
		if extensionName == name {
			return planner, nil
		}
	}
	return nil, nil
}

// resolveBlackboard picks the [core.Blackboard] for a fresh process —
// per-call value wins; otherwise the most-recently-registered
// [core.Blackboard] extension is used as a prototype, with [Clone]
// producing the isolated per-process instance; otherwise the built-in
// in-memory implementation is constructed fresh.
func (e *Engine) resolveBlackboard(supplied core.Blackboard) (core.Blackboard, error) {
	if supplied != nil {
		if _, err := blackboardName(supplied); err != nil {
			return nil, err
		}
		return supplied, nil
	}
	if prototype := e.blackboardPrototype(); prototype != nil {
		return cloneBlackboard(prototype)
	}
	return newInMemoryBlackboard(), nil
}

func blackboardName(blackboard core.Blackboard) (string, error) {
	if valueIsNil(blackboard) {
		return "", errors.New("blackboard is nil")
	}
	name, err := extensionName(blackboard)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("blackboard %T returned an empty Name", blackboard)
	}
	return name, nil
}

func cloneBlackboard(source core.Blackboard) (clone core.Blackboard, err error) {
	name, err := blackboardName(source)
	if err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			clone = nil
			err = panicerr.New(fmt.Sprintf("blackboard %q Clone panicked", name), recovered)
		}
	}()
	clone = source.Clone()
	if valueIsNil(clone) {
		return nil, fmt.Errorf("blackboard %q Clone returned nil", name)
	}
	if _, err := blackboardName(clone); err != nil {
		return nil, fmt.Errorf("blackboard %q Clone result: %w", name, err)
	}
	return clone, nil
}

func nextProcessID(generator core.IDGenerator) (id string, err error) {
	name, err := extensionName(generator)
	if err != nil {
		return "", err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			id = ""
			err = panicerr.New(fmt.Sprintf("ID generator %q Next panicked", name), recovered)
		}
	}()
	id = generator.Next()
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
		return "", fmt.Errorf("%w: ID generator %q returned %q", ErrProcessIdentity, name, id)
	}
	return id, nil
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
		name, err := extensionName(extension)
		if err != nil {
			return fmt.Errorf("ProcessOptions.Extensions[%d]: %w", index, err)
		}
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
func bindBlackboardSeed(blackboard core.Blackboard, bindings core.Bindings) (err error) {
	if bindings.Len() == 0 {
		return nil
	}
	name, err := blackboardName(blackboard)
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("blackboard %q seed panicked", name), recovered)
		}
	}()
	for key, value := range bindings.All() {
		if key == core.DefaultBindingName {
			blackboard.Bind(value)
			continue
		}
		blackboard.Store(key, value)
	}
	return nil
}
