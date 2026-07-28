package workflow

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// TeamConfig describes agents whose planning definitions should be composed
// into one agent. Action, goal, and condition names must be unique across all
// members.
type TeamConfig struct {
	Name        string
	Description string
	Agents      []*core.Agent
	PlannerName string
}

// Team composes several agent definitions into one ordinary agent. The result
// follows the same deployment and execution path as every other agent; Team
// introduces no second runtime model.
func Team(config TeamConfig) (*core.Agent, error) {
	if config.Name == "" {
		return nil, errors.New("workflow.Team: Name must not be empty")
	}
	if len(config.Agents) == 0 {
		return nil, errors.New("workflow.Team: Agents must not be empty")
	}

	var (
		actions       []core.Action
		goals         []*core.Goal
		conditions    []core.Condition
		snapshotState []core.Binding
	)
	for index, agent := range config.Agents {
		if agent == nil {
			return nil, fmt.Errorf("workflow.Team: Agents[%d] is nil", index)
		}
		actions = append(actions, agent.Actions()...)
		goals = append(goals, agent.Goals()...)
		conditions = append(conditions, agent.Conditions()...)
		snapshotState = append(snapshotState, agent.SnapshotState()...)
	}

	team := core.NewAgent(core.AgentConfig{
		Name:          config.Name,
		Description:   config.Description,
		Actions:       actions,
		Goals:         goals,
		Conditions:    conditions,
		SnapshotState: snapshotState,
		PlannerName:   config.PlannerName,
	})
	if err := team.Validate(); err != nil {
		return nil, fmt.Errorf("workflow.Team: %w", err)
	}
	return team, nil
}
