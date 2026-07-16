package core

// TerminationScope identifies the boundary affected by a termination request.
type TerminationScope int

const (
	// TerminationScopeAgent stops the entire process.
	TerminationScopeAgent TerminationScope = iota

	// TerminationScopeAction stops the current action and replans.
	TerminationScopeAction
)

func (s TerminationScope) String() string {
	switch s {
	case TerminationScopeAgent:
		return "agent"
	case TerminationScopeAction:
		return "action"
	default:
		return "unknown"
	}
}

// TerminationSignal is a pending structured termination request.
type TerminationSignal struct {
	Scope  TerminationScope
	Reason string
}
