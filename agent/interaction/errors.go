package interaction

import "errors"

var (
	ErrInvalidDefinitionConfig = errors.New("interaction: invalid definition configuration")
	ErrInvalidDispatcherConfig = errors.New("interaction: invalid dispatcher configuration")
	ErrInvalidDelegate         = errors.New("interaction: invalid delegate")
	ErrInvalidArtifact         = errors.New("interaction: invalid artifact")
	ErrInvalidInput            = errors.New("interaction: invalid input")
	ErrInvalidExecutionState   = errors.New("interaction: invalid execution state")
)
