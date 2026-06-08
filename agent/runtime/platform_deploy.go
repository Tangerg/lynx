package runtime

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// Deploy registers an agent after a multi-layer validation that reports
// every problem at once (embabel-style) rather than stopping at the first:
//
//  1. [core.Agent.Validate] checks structural invariants.
//  2. [checkGoalsReachable] does a one-step producer scan for GOAP so
//     every unreachable goal fails at deploy time, not first tick.
//  3. Every [core.AgentValidator] extension runs; each error is collected
//     (with the validator's Name attributed).
//
// All collected problems are joined into a single error so a misconfigured
// agent surfaces its full problem list in one deploy attempt.
//
// Re-deploying with the same name replaces the previous registration
// — convenient when iterating during development.
func (p *Platform) Deploy(a *core.Agent) error {
	if err := p.validateForDeploy(a); err != nil {
		return err
	}

	p.agents.register(a)
	p.publish(event.AgentDeployed{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: a.Name,
	})
	return nil
}

// validateForDeploy aggregates the structural, reachability, and
// extension-validator checks into one [errors.Join]ed error (nil when the
// agent is valid). A nil agent short-circuits since the later layers can't
// run without one.
func (p *Platform) validateForDeploy(a *core.Agent) error {
	if a == nil {
		return fmt.Errorf("runtime.Platform.validateForDeploy: deploy agent: %w", a.Validate())
	}

	var problems []error
	if err := a.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("structure: %w", err))
	}
	problems = append(problems, checkGoalsReachable(a)...)
	problems = append(problems, runAgentValidators(collectExtensions[core.AgentValidator](p.extensions.list), a)...)

	if joined := errors.Join(problems...); joined != nil {
		return fmt.Errorf("runtime.Platform.validateForDeploy: deploy agent %q: %w", a.Name, joined)
	}
	return nil
}

// Undeploy removes an agent. Returns an error when the name is
// unknown so callers don't silently miss typos.
func (p *Platform) Undeploy(name string) error {
	if err := p.agents.unregister(name); err != nil {
		return fmt.Errorf("runtime.Platform.Undeploy: undeploy agent %q: %w", name, err)
	}
	p.publish(event.AgentUndeployed{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: name,
	})
	return nil
}

// checkGoalsReachable does a conservative one-step producer scan: for
// every condition each goal requires, verify that either an action's
// effects can establish it OR an action's input binding looks like
// it (input bindings are externally supplied via process bindings +
// dual-binding rules). It returns one error per unreachable
// (goal, condition) pair so Deploy can report them all together.
//
// Intentionally weaker than running the full planner from empty
// state — the latter falsely rejects agents whose first action's
// precondition is "input binding present", because empty world state
// has no bindings. We accept the false-negative tradeoff so
// legitimate input-driven agents can deploy. Nil actions/goals are
// skipped here; structural validation reports those separately.
func checkGoalsReachable(a *core.Agent) []error {
	producible := map[string]struct{}{}
	for _, action := range a.Actions {
		if action == nil {
			continue
		}
		meta := action.Metadata()
		for key, value := range meta.Effects {
			if value == core.True {
				producible[key] = struct{}{}
			}
		}
		for _, in := range meta.Inputs {
			producible[in.String()] = struct{}{}
		}
	}

	var problems []error
	for _, goal := range a.Goals {
		if goal == nil {
			continue
		}
		for key, required := range goal.Preconditions() {
			if required != core.True {
				continue
			}
			if _, ok := producible[key]; !ok {
				problems = append(problems, fmt.Errorf(
					"runtime.checkGoalsReachable: goal %q requires condition %q, but no action produces it",
					goal.Name, key,
				))
			}
		}
	}
	return problems
}

// idGenerator returns the most-recently-registered IDGenerator
// extension, falling back to a UUID-v4 generator when none is
// registered.
func (p *Platform) idGenerator() core.IDGenerator {
	if g := lastExtension[core.IDGenerator](p.extensions.list); g != nil {
		return g
	}
	return defaultIDGenerator
}

// blackboardPrototype returns the most-recently-registered
// [core.Blackboard] extension or nil. The runtime treats it as a
// prototype: every new process gets its own instance via
// [core.Blackboard.Spawn] so per-process state stays isolated. Callers
// fall back to the in-memory implementation when nil.
func (p *Platform) blackboardPrototype() core.Blackboard {
	return lastExtension[core.Blackboard](p.extensions.list)
}

// Built-in fallback for the IDGenerator singleton. Planner resolution
// uses [Platform.resolvePlanner] (name-based dispatch over registered
// extensions, with goap / reactive built-in defaults); Blackboard
// resolution uses [Platform.blackboardPrototype] + Spawn().
var defaultIDGenerator = core.NewUUIDIDGenerator("")
