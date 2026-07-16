package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// goalSummary is the wire shape for a *core.Goal — lossy on the
// non-serializable fields ([core.ScoreFunc] callbacks can't round-trip).
type goalSummary struct {
	Name          string         `json:"name,omitempty"`
	Description   string         `json:"description,omitempty"`
	Preconditions []string       `json:"pre,omitempty"`
	Inputs        []core.Binding `json:"inputs,omitempty"`
}

func summarizeGoal(goal *core.Goal) *goalSummary {
	if goal == nil {
		return nil
	}
	return &goalSummary{
		Name:          goal.Name(),
		Description:   goal.Description(),
		Preconditions: goal.RequiredConditions(),
		Inputs:        goal.Inputs(),
	}
}

// actionName returns the action's name, or "" when nil.
func actionName(action core.Action) string {
	if action == nil {
		return ""
	}
	return action.Metadata().Name
}

// actionNames maps the action slice to its names. Used by plan summaries.
func actionNames(actions []core.Action) []string {
	if len(actions) == 0 {
		return nil
	}
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, actionName(action))
	}
	return names
}

// planSummary is the wire shape for *planning.Plan — actions reduce to the
// ordered list of their names, goal becomes a goalSummary.
type planSummary struct {
	Actions []string     `json:"actions,omitempty"`
	Goal    *goalSummary `json:"goal,omitempty"`
}

func summarizePlan(plan *planning.Plan) *planSummary {
	if plan == nil {
		return nil
	}
	return &planSummary{
		Actions: actionNames(plan.Actions()),
		Goal:    summarizeGoal(plan.Goal()),
	}
}

// worldStateSummary captures everything serializable from a [core.WorldState]:
// the condition map and the snapshot timestamp.
type worldStateSummary struct {
	State     map[string]core.Truth `json:"state,omitempty"`
	Timestamp time.Time             `json:"timestamp,omitzero"`
}

func summarizeWorldState(state core.WorldState) *worldStateSummary {
	if state == nil {
		return nil
	}
	return &worldStateSummary{State: state.Conditions(), Timestamp: state.Timestamp()}
}

func summarizeValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return fmt.Sprint(value)
	}
	return decoded
}

func summarizeMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	summary := make(map[string]any, len(values))
	for key, value := range values {
		summary[key] = summarizeValue(value)
	}
	return summary
}
