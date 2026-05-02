package core

// ResponseImpact tells the runtime whether the human's reply actually changed
// the world state. UNCHANGED leaves the blackboard alone (used when the user
// confirms current state); UPDATED triggers a fresh planning tick because
// preconditions might now be satisfied.
type ResponseImpact int8

const (
	ResponseImpactUnchanged ResponseImpact = iota
	ResponseImpactUpdated
)

// Awaitable is the non-generic root of HITL prompts; it lives here so Process
// can declare AwaitInput without dragging the hitl package into core. The
// hitl/ subpackage provides the typed Awaitable[P, R] generic interface plus
// concrete request types.
type Awaitable interface {
	// ID is a stable identifier — used by Platform.ResumeProcess to look up
	// the pending request.
	ID() string

	// PromptAny returns the payload to display (form schema, confirmation
	// message, etc.) in its untyped form. Generic implementations expose a
	// typed Prompt() too; the untyped accessor is what the runtime persists.
	PromptAny() any
}
