package runtime

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// createProcess assembles an AgentProcess and its dependencies
// (blackboard, determiner, planner). The process is registered in
// the platform's map before being returned so concurrent
// ResumeProcess / KillProcess calls can find it.
func (p *Platform) createProcess(
	agentDef *core.Agent,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if agentDef == nil {
		return nil, errors.New("runtime.Platform.createProcess: agent definition is nil")
	}
	if err := agentDef.Validate(); err != nil {
		return nil, fmt.Errorf("runtime.Platform.createProcess: %w", err)
	}
	if err := validateProcessExtensions(options.Extensions); err != nil {
		return nil, fmt.Errorf("runtime.Platform.createProcess: %w", err)
	}
	options.ApplyDefaults()

	blackboard := p.resolveBlackboard(options.Blackboard)
	bindBlackboardSeed(blackboard, bindings)

	planner, err := p.resolvePlanner(agentDef, options.Extensions)
	if err != nil {
		return nil, err
	}

	system := planning.FromAgent(agentDef)
	id := p.idGenerator().Next()
	proc := newAgentProcess(id, agentDef, &options, blackboard, planner, system, p)

	// determiner + per-process event multicast both close over the
	// assembled pointer, so they're wired after construction.
	proc.wireRuntimeDeps(options.Extensions)

	p.procs.register(proc)
	p.publish(event.ProcessCreated{
		BaseEvent: event.NewBaseEvent(id),
		Bindings:  bindings,
	})
	return proc, nil
}

// CreateChildProcess spawns a sub-agent process whose blackboard
// inherits the parent's. Used by composite agents that delegate
// sub-tasks; budget aggregation happens automatically through
// [(*AgentProcess).Usage]'s recursive walk.
func (p *Platform) CreateChildProcess(
	agentDef *core.Agent,
	parent *AgentProcess,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if parent == nil {
		return nil, errors.New("runtime.Platform.CreateChildProcess: parent process is nil")
	}
	if options.Blackboard == nil {
		options.Blackboard = parent.Blackboard().Spawn()
	}
	// A child shares its parent's event stream: process-scope
	// EventListener extensions propagate down so the whole delegation
	// subtree surfaces on the listener the parent registered (each event
	// keeps its own ProcessID, so a consumer can tell parent from child).
	// Listeners are the only capability inherited — blackboard / planner /
	// tool extensions stay scoped to the process that declared them. No-op
	// when the parent registered no listeners, so the historical "child
	// events reach only the platform multicast" behavior is unchanged for
	// callers that don't observe per-process.
	options.Extensions = inheritEventListeners(options.Extensions, parent.options)

	child, err := p.createProcess(agentDef, nil, options)
	if err != nil {
		return nil, err
	}
	child.parentID = parent.id
	parent.budget.addChild(child)
	return child, nil
}

// inheritEventListeners propagates a parent's process-scope
// [EventListener] extensions onto a child's option set so the whole
// delegation subtree feeds the listener the parent registered. Only
// values implementing EventListener propagate — other capabilities
// (Blackboard / Planner / ToolDecorator …) stay scoped to the process
// that declared them. A child that already declares a listener of the
// same Name wins (its own is not shadowed) and duplicates are skipped so
// validateProcessExtensions' uniqueness check still holds. Returns the
// child slice unchanged when the parent registered no extensions.
func inheritEventListeners(childExts []core.Extension, parent *core.ProcessOptions) []core.Extension {
	if parent == nil || len(parent.Extensions) == 0 {
		return childExts
	}
	seen := make(map[string]struct{}, len(childExts))
	for _, ext := range childExts {
		if ext != nil {
			seen[ext.Name()] = struct{}{}
		}
	}
	for _, ext := range parent.Extensions {
		if ext == nil {
			continue
		}
		if _, ok := ext.(EventListener); !ok {
			continue
		}
		if _, dup := seen[ext.Name()]; dup {
			continue
		}
		childExts = append(childExts, ext)
		seen[ext.Name()] = struct{}{}
	}
	return childExts
}

// resolvePlanner finds the [planning.Planner] for agentDef by matching
// [core.AgentConfig.PlannerName] against registered Planner extensions:
// process-scope extensions take priority over platform-scope. An empty
// PlannerName resolves to "goap". The runtime intentionally knows no
// concrete planner — the composition root registers them as extensions
// (agent.NewPlatform registers goap + reactive by default).
func (p *Platform) resolvePlanner(agentDef *core.Agent, processExts []core.Extension) (planning.Planner, error) {
	name := agentDef.PlannerName
	if name == "" {
		name = "goap"
	}

	if planner := findPlannerByName(processExts, name); planner != nil {
		return planner, nil
	}
	if planner := findPlannerByName(p.extensions.list, name); planner != nil {
		return planner, nil
	}

	return nil, fmt.Errorf("runtime.Platform.resolvePlanner: agent %q requests planner %q which is not registered — register a planning.Planner extension with that Name (agent.NewPlatform registers goap + reactive by default)", agentDef.Name, name)
}

// findPlannerByName walks extensions for a [planning.Planner] whose
// Name() matches. Returns nil when none matches.
func findPlannerByName(extensions []core.Extension, name string) planning.Planner {
	for _, ext := range extensions {
		planner, ok := ext.(planning.Planner)
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
// [core.Blackboard] extension is used as a prototype, with [Spawn]
// producing the isolated per-process instance; otherwise the built-in
// in-memory implementation is constructed fresh.
func (p *Platform) resolveBlackboard(supplied core.Blackboard) core.Blackboard {
	if supplied != nil {
		return supplied
	}
	if prototype := p.blackboardPrototype(); prototype != nil {
		if bb := prototype.Spawn(); bb != nil {
			return bb
		}
	}
	return newInMemoryBlackboard()
}

// validateProcessExtensions enforces the per-process invariants:
// nil rejected, empty Names rejected, no duplicate Names within the
// slice. Process-scope Names ARE allowed to collide with
// platform-scope Names — that's the explicit override mechanism.
func validateProcessExtensions(extensions []core.Extension) error {
	if len(extensions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(extensions))
	for i, ext := range extensions {
		if ext == nil {
			return fmt.Errorf("ProcessOptions.Extensions[%d] is nil", i)
		}
		name := ext.Name()
		if name == "" {
			return fmt.Errorf("ProcessOptions.Extensions[%d] (%T) returned empty Name()", i, ext)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("ProcessOptions.Extensions: duplicate name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// bindBlackboardSeed applies the caller's initial bindings.
// [core.DefaultBindingName] uses Bind() so the dual-binding behavior
// kicks in; other keys go through Set so their explicit name wins.
func bindBlackboardSeed(blackboard core.Blackboard, bindings map[string]any) {
	for key, value := range bindings {
		if key == core.DefaultBindingName {
			blackboard.Bind(value)
			continue
		}
		blackboard.Set(key, value)
	}
}
