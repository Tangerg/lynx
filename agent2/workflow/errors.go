package workflow

import "errors"

var (
	// ErrInvalidStage reports an incomplete or contradictory Stage.
	ErrInvalidStage = errors.New("workflow: invalid stage")

	// ErrInvalidDefinitionConfig reports a malformed Workflow Definition.
	ErrInvalidDefinitionConfig = errors.New("workflow: invalid definition configuration")

	// ErrInvalidExecutionState reports malformed or inconsistent Workflow state.
	ErrInvalidExecutionState = errors.New("workflow: invalid execution state")

	// ErrInvalidProtocol reports an unexpected child Effect or Signal contract.
	ErrInvalidProtocol = errors.New("workflow: invalid protocol payload")
)
