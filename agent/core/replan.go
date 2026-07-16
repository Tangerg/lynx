package core

// ReplanRequest tells the runtime that an action invalidated the current plan.
// The runtime excludes that action for one tick, applies Update, and plans
// again.
type ReplanRequest struct {
	Reason string

	// Update stages state discovered by the action before re-planning.
	Update func(Blackboard)
}

func (r *ReplanRequest) Error() string {
	if r == nil || r.Reason == "" {
		return "replan requested"
	}
	return "replan requested: " + r.Reason
}
