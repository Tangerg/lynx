package runtime

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
	"github.com/Tangerg/lynx/agent/plan/planner/goap"
	"github.com/Tangerg/lynx/agent/plan/planner/reactive"
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
		return nil, errors.New("create process: agent definition is nil")
	}
	if err := core.ValidateAgent(agentDef); err != nil {
		return nil, fmt.Errorf("create process: %w", err)
	}
	if err := validateProcessExtensions(options.Extensions); err != nil {
		return nil, fmt.Errorf("create process: %w", err)
	}
	options.ApplyDefaults()

	blackboard := p.resolveBlackboard(options.Blackboard)
	bindBlackboardSeed(blackboard, bindings)

	planner, err := p.resolvePlanner(agentDef, options.Extensions)
	if err != nil {
		return nil, err
	}

	system := plan.FromAgent(agentDef)
	id := p.idGenerator().Next()
	proc := newAgentProcess(id, agentDef, &options, blackboard, planner, system, p)

	// determiner needs the *AgentProcess pointer (for user-defined
	// conditions); processEvents subscribes process-scope
	// EventListener extensions so they only see this process's
	// events.
	proc.determiner = newBlackboardDeterminer(system, blackboard, proc)
	proc.processEvents = event.NewMulticast()
	addEventListenerExtensions(proc.processEvents, options.Extensions)

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
		return nil, errors.New("create child process: parent process is nil")
	}
	if options.Blackboard == nil {
		options.Blackboard = parent.Blackboard().Spawn()
	}

	child, err := p.createProcess(agentDef, nil, options)
	if err != nil {
		return nil, err
	}
	child.parentID = parent.id
	parent.budget.addChild(child)
	return child, nil
}

// resolvePlanner finds the [plan.Planner] for agentDef by matching
// [core.AgentConfig.PlannerName] against registered Planner extensions
// (process-scope extensions take priority over platform-scope, then
// the framework default). An empty PlannerName resolves to "goap".
func (p *Platform) resolvePlanner(agentDef *core.Agent, processExts []core.Extension) (plan.Planner, error) {
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
	if planner := defaultPlanner(name); planner != nil {
		return planner, nil
	}

	hint := ""
	if name == "htn" {
		hint = " — register an *htn.Planner extension built with your task library"
	}
	return nil, fmt.Errorf("runtime.Platform.resolvePlanner: agent %q requests planner %q which is not registered%s", agentDef.Name, name, hint)
}

// findPlannerByName walks extensions for a [plan.Planner] whose
// Name() matches. Returns nil when none matches.
func findPlannerByName(extensions []core.Extension, name string) plan.Planner {
	for _, ext := range extensions {
		planner, ok := ext.(plan.Planner)
		if !ok {
			continue
		}
		if planner.Name() == name {
			return planner
		}
	}
	return nil
}

// defaultPlanner returns the framework's built-in planner for name,
// or nil if name does not match a framework default. "htn" is not
// here because it needs a user-supplied task library.
func defaultPlanner(name string) plan.Planner {
	switch name {
	case "goap":
		return goap.NewAStarPlanner()
	case "reactive":
		return reactive.NewPlanner()
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
// [core.DefaultBindingName] uses Bind() so the dual-binding behaviour
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
