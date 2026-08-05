package protocol

import "context"

// Plan is the plan.* method group — the cold read behind the plan state key.
type Plan interface {
	// GetPlan returns the session's Plan projection, unchanged from what the
	// stream publishes. A session with no list yet is the empty state at revision 0;
	// only a session that does not exist is session_not_found.
	GetPlan(ctx context.Context, in GetPlanRequest) (*StateSnapshot, error)
}
