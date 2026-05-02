package core

import "errors"

// ReplanRequest is the Go-flavored replacement for embabel's
// ReplanRequestedException. An action that decides "what I just learned
// invalidates the current plan" returns one as an error; the runtime catches
// it via errors.As, blacklists the offending action for one tick, and
// reformulates the plan.
type ReplanRequest struct {
	Reason string

	// Update runs before re-planning so the action can stage the change
	// (e.g. "set route=alternate") that motivated the re-plan.
	Update func(Blackboard)
}

func (r *ReplanRequest) Error() string {
	if r == nil || r.Reason == "" {
		return "replan requested"
	}
	return "replan requested: " + r.Reason
}

// AsReplanRequest returns the embedded *ReplanRequest if err carries one,
// nil otherwise. Mirrors errors.As ergonomics without forcing callers to
// pass a typed pointer.
func AsReplanRequest(err error) *ReplanRequest {
	if err == nil {
		return nil
	}

	var rr *ReplanRequest
	if errors.As(err, &rr) {
		return rr
	}
	return nil
}
