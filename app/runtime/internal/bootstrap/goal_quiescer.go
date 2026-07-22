package bootstrap

import "github.com/Tangerg/lynx/app/runtime/internal/application/goals"

// goalQuiescerRef late-binds the Goal driver into the session lifecycle
// coordinator. The coordinator must quiesce a session's autonomous goal loop
// before a delete/rollback/restore clears its goal, but the driver is
// constructed after the coordinator: the driver depends on the run coordinator,
// which depends on the session coordinator. This holder satisfies the
// coordinator's GoalQuiescer port at construction and is populated with the
// driver once it exists — before any request is served — breaking the
// construction cycle without a public setter on the coordinator.
type goalQuiescerRef struct{ d *goals.Driver }

func (r *goalQuiescerRef) Quiesce(sessionID string) {
	if r.d != nil {
		r.d.Quiesce(sessionID)
	}
}
