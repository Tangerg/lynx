package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// ScopeConfig is the configuration for [Platform.RunInScope]. It
// fuses every listed agent's actions, goals, and conditions into a
// single planning universe and asks the planner to pick a path across
// that union.
//
// Action / goal / condition names must be unique across the scope —
// duplicate names are rejected at validation time (same rule as for a
// regular [core.Agent]). Callers prefix names per agent (e.g.
// "checkout:authorize", "checkout:capture", "fulfillment:pack") to
// avoid collisions while keeping intent legible.
type ScopeConfig struct {
	// Name is the synthetic scope agent's identifier — surfaces in
	// traces and lookups as `Platform.FindAgent(Name)`. Required, must
	// not collide with an already-deployed agent.
	Name string

	// Description is the human-readable scope summary surfaced in
	// tracing. Optional.
	Description string

	// Agents are the participants. Must be non-empty. Nil entries are
	// rejected so missing wiring fails fast.
	Agents []*core.Agent

	// PlannerName picks the joint planner. Empty → framework default
	// ("goap"). Built-in names: "goap", "reactive". Custom planners
	// must be registered as [core.Extension]s.
	PlannerName string
}

// RunInScope is the joint-planning entry point: the runtime builds a
// synthetic "scope agent" out of the union of every listed agent's
// actions / goals / conditions, deploys it transparently (if not
// already deployed), and dispatches it through the same [RunAgent]
// path as any other agent. The planner therefore reasons over the
// whole capability bag and may pick an action from one agent followed
// by an action from another — joint planning across the fused scope.
//
// The synthetic scope agent is re-used across calls with the same
// [ScopeConfig.Name] so repeated invocations don't churn registry state.
// To refresh after the participant set changes, call
// [Platform.Undeploy] on the scope name first.
//
// Errors flow from validation (empty Agents, nil entry, duplicate
// names within the union) and from the dispatch (planner failures,
// missing tool groups, etc).
func (p *Platform) RunInScope(
	ctx context.Context,
	cfg ScopeConfig,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if cfg.Name == "" {
		return nil, errors.New("run in scope: Name must not be empty")
	}
	if len(cfg.Agents) == 0 {
		return nil, errors.New("run in scope: Agents must not be empty")
	}
	for i, a := range cfg.Agents {
		if a == nil {
			return nil, fmt.Errorf("run in scope: Agents[%d] is nil", i)
		}
	}

	scope, err := p.resolveScopeAgent(cfg)
	if err != nil {
		return nil, err
	}
	return p.RunAgent(ctx, scope, bindings, options)
}

// resolveScopeAgent returns the synthetic scope agent for cfg, building
// and deploying it the first time it's seen. Subsequent calls with the
// same [ScopeConfig.Name] reuse the deployed instance — the scope agent
// is cached across invocations.
func (p *Platform) resolveScopeAgent(cfg ScopeConfig) (*core.Agent, error) {
	if existing, ok := p.agents.find(cfg.Name); ok {
		return existing, nil
	}

	scope := BuildScopeAgent(cfg)
	if err := p.Deploy(scope); err != nil {
		return nil, fmt.Errorf("run in scope: deploy synthetic scope agent %q: %w", cfg.Name, err)
	}
	return scope, nil
}

// BuildScopeAgent constructs the synthetic scope [core.Agent] without
// deploying it — exposed so callers that want to validate / inspect a
// scope before running can do so, and so tests can exercise the
// builder directly.
//
// Action / goal / condition slices come straight from the input agents
// in input order; the resulting agent must pass [core.Agent.Validate]
// for [Platform.Deploy] to accept it (so unique names across the union
// is a hard requirement at deploy time).
func BuildScopeAgent(cfg ScopeConfig) *core.Agent {
	var (
		actions    []core.Action
		goals      []*core.Goal
		conditions []core.Condition
	)
	for _, a := range cfg.Agents {
		if a == nil {
			continue
		}
		actions = append(actions, a.Actions...)
		goals = append(goals, a.Goals...)
		conditions = append(conditions, a.Conditions...)
	}

	description := cfg.Description
	if description == "" {
		description = fmt.Sprintf("synthetic scope across %d agent(s)", len(cfg.Agents))
	}

	return core.NewAgent(core.AgentConfig{
		Name:        cfg.Name,
		Description: description,
		Actions:     actions,
		Goals:       goals,
		Conditions:  conditions,
		PlannerName: cfg.PlannerName,
	})
}
