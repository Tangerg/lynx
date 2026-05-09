package runtime

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// Deploy registers an agent after a 3-layer validation:
//
//  1. [core.ValidateAgent] checks structural invariants.
//  2. [checkGoalsReachable] does a one-step producer scan for GOAP
//     so unreachable goals fail at deploy time, not first tick.
//  3. Every [core.AgentValidator] extension runs in registration
//     order; the first error vetoes (with the validator's Name
//     attributed).
//
// Re-deploying with the same name replaces the previous registration
// — convenient when iterating during development.
func (p *Platform) Deploy(a *core.Agent) error {
	if err := core.ValidateAgent(a); err != nil {
		return fmt.Errorf("deploy agent: %w", err)
	}
	if err := checkGoalsReachable(a); err != nil {
		return fmt.Errorf("deploy agent %q: %w", a.Name, err)
	}
	if err := runAgentValidators(collectExtensions[core.AgentValidator](p.extensions.list), a); err != nil {
		return fmt.Errorf("deploy agent %q: %w", a.Name, err)
	}

	p.agents.register(a)
	p.publish(event.AgentDeployed{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: a.Name,
	})
	return nil
}

// Undeploy removes an agent. Returns an error when the name is
// unknown so callers don't silently miss typos.
func (p *Platform) Undeploy(name string) error {
	if err := p.agents.unregister(name); err != nil {
		return fmt.Errorf("undeploy agent %q: %w", name, err)
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
// dual-binding rules).
//
// Intentionally weaker than running the full planner from empty
// state — the latter falsely rejects agents whose first action's
// precondition is "input binding present", because empty world state
// has no bindings. We accept the false-negative tradeoff so
// legitimate input-driven agents can deploy.
func checkGoalsReachable(a *core.Agent) error {
	producible := map[string]struct{}{}
	for _, action := range a.Actions {
		if action == nil {
			return fmt.Errorf("action list contains a nil action")
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

	for _, goal := range a.Goals {
		for key, required := range goal.Preconditions() {
			if required != core.True {
				continue
			}
			if _, ok := producible[key]; !ok {
				return fmt.Errorf(
					"goal %q requires condition %q, but no action produces it",
					goal.Name, key,
				)
			}
		}
	}
	return nil
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

// plannerFactory mirrors idGenerator for PlannerFactory.
func (p *Platform) plannerFactory() PlannerFactory {
	if f := lastExtension[PlannerFactory](p.extensions.list); f != nil {
		return f
	}
	return defaultPlannerFactoryInstance
}

// blackboardFactory returns the most-recently-registered
// BlackboardFactory extension or nil — callers fall back to the
// in-memory blackboard.
func (p *Platform) blackboardFactory() core.BlackboardFactory {
	return lastExtension[core.BlackboardFactory](p.extensions.list)
}

// Built-in fallbacks for last-wins singletons.
var (
	defaultIDGenerator            = core.NewUUIDIDGenerator("")
	defaultPlannerFactoryInstance = DefaultPlannerFactory()
)
