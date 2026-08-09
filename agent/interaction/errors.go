package interaction

import "errors"

var (
	// ErrInvalidDefinitionConfig reports an incomplete or contradictory
	// Interaction Definition configuration.
	ErrInvalidDefinitionConfig = errors.New("interaction: invalid definition configuration")
	// ErrInvalidDispatcherConfig reports an unusable model or Tool binding.
	ErrInvalidDispatcherConfig = errors.New("interaction: invalid dispatcher configuration")
	// ErrInvalidDelegate reports an unusable model-facing child binding.
	ErrInvalidDelegate = errors.New("interaction: invalid delegate")
	// ErrInvalidArtifact reports a malformed or undecodable successful
	// Delegate output value.
	ErrInvalidArtifact = errors.New("interaction: invalid artifact")
	// ErrInvalidInput reports malformed managed Interaction input.
	ErrInvalidInput = errors.New("interaction: invalid input")
	// ErrInvalidExecutionState reports malformed or inconsistent Interaction
	// Execution state.
	ErrInvalidExecutionState = errors.New("interaction: invalid execution state")
)
