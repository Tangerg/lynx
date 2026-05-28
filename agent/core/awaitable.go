package core

// ResponseImpact tells the runtime whether the human's reply actually changed
// the world state. UNCHANGED leaves the blackboard alone (used when the user
// confirms current state); UPDATED triggers a fresh planning tick because
// preconditions might now be satisfied.
type ResponseImpact int8

const (
	ImpactUnchanged ResponseImpact = iota
	ImpactUpdated
)

// Awaitable is the non-generic root of HITL prompts; it lives here so
// [Process] can declare [Process.AwaitInput] without dragging the hitl
// package into core. The hitl/ subpackage provides the typed
// [hitl.Request][P, R] interface plus the canonical
// [hitl.TypedRequest] implementation (with [hitl.NewConfirmation] as the
// boolean-response specialisation).
type Awaitable interface {
	// ID is a stable identifier — used by [Platform.ResumeProcess] to
	// look up the pending request.
	ID() string

	// PromptAny returns the payload to display (form schema, confirmation
	// message, etc.) in its untyped form. Generic implementations expose
	// a typed Prompt() too; the untyped accessor is what the runtime
	// persists and serializes.
	PromptAny() any

	// OnResponseAny accepts an untyped response and routes it to the
	// implementation's typed handler. Returns the [ResponseImpact] the
	// handler decided plus an error when the response value isn't
	// assignable to the awaitable's expected response type.
	//
	// Typed flavors (e.g. [hitl.TypedRequest]) implement this by
	// type-asserting response to their concrete R type and forwarding
	// to OnResponse(R).
	OnResponseAny(response any) (ResponseImpact, error)
}
