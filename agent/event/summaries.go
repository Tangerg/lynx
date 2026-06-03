package event

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// goalSummary is the wire shape for a *core.Goal — lossy on the
// non-serializable fields ([core.CostFunc] callbacks can't round-trip).
type goalSummary struct {
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Pre         []string         `json:"pre,omitempty"`
	Inputs      []core.IOBinding `json:"inputs,omitempty"`
}

func summarizeGoal(g *core.Goal) *goalSummary {
	if g == nil {
		return nil
	}
	return &goalSummary{
		Name:        g.Name,
		Description: g.Description,
		Pre:         g.Pre,
		Inputs:      g.Inputs,
	}
}

// actionName returns the action's name, or "" when nil.
func actionName(a core.Action) string {
	if a == nil {
		return ""
	}
	return a.Metadata().Name
}

// actionNames maps the action slice to its names. Used by plan summaries.
func actionNames(actions []core.Action) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, actionName(a))
	}
	return out
}

// planSummary is the wire shape for *planning.Plan — actions reduce to the
// ordered list of their names, goal becomes a goalSummary.
type planSummary struct {
	Actions []string     `json:"actions,omitempty"`
	Goal    *goalSummary `json:"goal,omitempty"`
}

func summarizePlan(p *planning.Plan) *planSummary {
	if p == nil {
		return nil
	}
	return &planSummary{
		Actions: actionNames(p.Actions),
		Goal:    summarizeGoal(p.Goal),
	}
}

// worldSnapshot captures everything serializable from a [core.WorldState]:
// the condition map and the snapshot timestamp.
type worldSnapshot struct {
	State     map[string]core.Determination `json:"state,omitempty"`
	Timestamp time.Time                     `json:"timestamp,omitzero"`
}

func snapshotWorld(ws core.WorldState) *worldSnapshot {
	if ws == nil {
		return nil
	}
	return &worldSnapshot{State: ws.State(), Timestamp: ws.Timestamp()}
}

// awaitableSummary is the wire shape for [core.Awaitable]: the stable id
// and the (untyped) prompt payload. Concrete prompt types serialize via
// their own MarshalJSON / struct tags; opaque payloads end up as the
// closest JSON form encoding/json can produce.
type awaitableSummary struct {
	ID     string `json:"id,omitempty"`
	Prompt any    `json:"prompt,omitempty"`
}

func summarizeAwaitable(a core.Awaitable) *awaitableSummary {
	if a == nil {
		return nil
	}
	return &awaitableSummary{ID: a.ID(), Prompt: a.PromptAny()}
}
