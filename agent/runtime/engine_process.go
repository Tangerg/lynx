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
	process, eventBindings, err := e.buildProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		return nil, core.Bindings{}, err
	}
	if !e.processes.insert(process) {
		return nil, core.Bindings{}, fmt.Errorf("runtime.Engine.createProcessFromDeployment: %w: duplicate ID %q", ErrProcessIdentity, process.id)
	}
	return process, eventBindings, nil
}

func (e *Engine) buildProcessFromDeployment(
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
	return e.Deploy(ctx, agent)
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
	// SubtreeEventListener extensions propagate down so the whole delegation
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

	child, eventBindings, err := e.buildProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		return nil, core.Bindings{}, err
	}
	if err := e.attachChild(parent, child); err != nil {
		return nil, core.Bindings{}, err
	}
	return child, eventBindings, nil
}

func (e *Engine) attachChild(parent, child *Process) error {
	parent.state.mu.Lock()
	defer parent.state.mu.Unlock()
	if parent.state.currentStatus != core.StatusRunning || !parent.state.runOwned {
		return fmt.Errorf("%w: parent process %q is %s", ErrChildParentInactive, parent.id, parent.state.currentStatus)
	}
	child.parentID = parent.id
	child.depth = parent.depth + 1
	if !e.processes.insert(child) {
		return fmt.Errorf("runtime.Engine.createChild: %w: duplicate ID %q", ErrProcessIdentity, child.id)
	}
	parent.budget.children = append(parent.budget.children, child)
	return nil
}

// resolvePlanner finds the [planning.Planner] for agent by matching
// [core.AgentConfig.PlannerName] against registered Planner extensions:
// process-scope extensions take priority over engine-scope. An empty
// PlannerName resolves through [planning.DefaultPlannerName]. The runtime intentionally knows no
// concrete planner — the composition root registers them as extensions
// (agent.NewEngine registers goap + reactive by default).
func (e *Engine) resolvePlanner(agent *core.Agent, processExtensions []extensionEntry) (planning.Planner, error) {
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
func findPlannerByName(extensions []extensionEntry, name string) (planning.Planner, error) {
	for _, extension := range extensions {
		planner, ok := extension.value.(planning.Planner)
		if !ok {
			continue
		}
		if extension.name == name {
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
	if prototype, ok := e.blackboardPrototype(); ok {
		return cloneBlackboardNamed(prototype.name, prototype.value)
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
	return cloneBlackboardNamed(name, source)
}

func cloneBlackboardNamed(name string, source core.Blackboard) (clone core.Blackboard, err error) {
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

func nextProcessID(generator extensionCapability[core.IDGenerator]) (id string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			id = ""
			err = panicerr.New(fmt.Sprintf("ID generator %q Next panicked", generator.name), recovered)
		}
	}()
	id = generator.value.Next()
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
		return "", fmt.Errorf("%w: ID generator %q returned %q", ErrProcessIdentity, generator.name, id)
	}
	return id, nil
}

// validateProcessExtensions enforces identity and scope before a Process keeps
// the extension instances. Process-scope Names may collide with engine-scope
// Names; selection capabilities treat that as an explicit override.
func registerProcessExtensions(extensions []core.Extension) ([]extensionEntry, error) {
	if len(extensions) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(extensions))
	registered := make([]extensionEntry, 0, len(extensions))
	for index, extension := range extensions {
		if valueIsNil(extension) {
			return nil, fmt.Errorf("ProcessOptions.Extensions[%d] is nil", index)
		}
		name, err := extensionName(extension)
		if err != nil {
			return nil, fmt.Errorf("ProcessOptions.Extensions[%d]: %w", index, err)
		}
		if name == "" {
			return nil, fmt.Errorf("ProcessOptions.Extensions[%d] (%T) returned empty Name()", index, extension)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("ProcessOptions.Extensions: duplicate name %q", name)
		}
		if err := validateProcessExtensionScope(extension); err != nil {
			return nil, fmt.Errorf("ProcessOptions.Extensions[%d] %q: %w", index, name, err)
		}
		seen[name] = struct{}{}
		registered = append(registered, extensionEntry{name: name, value: extension})
	}
	return registered, nil
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
