package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// TeamConfig is the configuration for [Engine.RunTeam]. It
// fuses every listed agent's actions, goals, and conditions into a
// single planning universe and asks the planner to pick a path across
// that union.
//
// Action / goal / condition names must be unique across the team —
// duplicate names are rejected at validation time (same rule as for a
// regular [core.Agent]). Callers prefix names per agent (e.g.
// "checkout:authorize", "checkout:capture", "fulfillment:pack") to
// avoid collisions while keeping intent legible.
type TeamConfig struct {
	// Name is the synthetic team agent's identifier — surfaces in
	// traces and active deployment lookups. Required, must
	// not collide with an already-deployed agent.
	Name string

	// Description is the human-readable team summary surfaced in
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

// RunTeam is the joint-planning entry point: the runtime builds a
// synthetic "team agent" out of the union of every listed agent's
// actions / goals / conditions, deploys it transparently, and dispatches it
// through the same [Engine.Run]
// path as any other agent. The planner therefore reasons over the
// whole capability bag and may pick an action from one agent followed
// by an action from another — joint planning across the fused team.
//
// Repeating the exact same synthetic definition is idempotent. Reusing a name
// for a changed definition returns [ErrDeploymentConflict]; callers must
// undeploy the old route before the next call.
//
// Errors flow from validation (empty Agents, nil entry, duplicate
// names within the union) and from the dispatch (planner failures,
// missing tool groups, etc).
func (e *Engine) RunTeam(
	ctx context.Context,
	config TeamConfig,
	bindings map[string]any,
	options core.ProcessOptions,
) (*Process, error) {
	if config.Name == "" {
		return nil, errors.New("run team: name must not be empty")
	}
	if len(config.Agents) == 0 {
		return nil, errors.New("run team: agents must not be empty")
	}
	for index, agent := range config.Agents {
		if agent == nil {
			return nil, fmt.Errorf("run team: agents[%d] is nil", index)
		}
	}

	team, err := e.resolveTeam(config)
	if err != nil {
		return nil, err
	}
	return e.Run(ctx, team, bindings, options)
}

// resolveTeam compiles config through the normal deployment catalog. The
// exact same team is idempotent; changing a team under an active name returns
// ErrDeploymentConflict instead of silently reusing a stale definition.
func (e *Engine) resolveTeam(config TeamConfig) (*core.Agent, error) {
	team := buildTeamAgent(config)
	deployment, err := e.Deploy(team)
	if err != nil {
		return nil, fmt.Errorf("run team: deploy synthetic agent %q: %w", config.Name, err)
	}
	return deployment.agent, nil
}

func buildTeamAgent(config TeamConfig) *core.Agent {
	var (
		actions    []core.Action
		goals      []*core.Goal
		conditions []core.Condition
	)
	for _, agent := range config.Agents {
		if agent == nil {
			continue
		}
		actions = append(actions, agent.Actions()...)
		goals = append(goals, agent.Goals()...)
		conditions = append(conditions, agent.Conditions()...)
	}

	description := config.Description
	if description == "" {
		description = fmt.Sprintf("synthetic team across %d agents", len(config.Agents))
	}

	return core.NewAgent(core.AgentConfig{
		Name:        config.Name,
		Description: description,
		Actions:     actions,
		Goals:       goals,
		Conditions:  conditions,
		PlannerName: config.PlannerName,
	})
}
