package planning

import "errors"

var (
	// ErrInvalidCondition reports a malformed Planning condition.
	ErrInvalidCondition = errors.New("planning: invalid condition")
	// ErrInvalidWorldState reports a malformed or non-canonical world state.
	ErrInvalidWorldState = errors.New("planning: invalid world state")
	// ErrInvalidGoal reports a malformed Planning goal.
	ErrInvalidGoal = errors.New("planning: invalid goal")
	// ErrInvalidAction reports a malformed Action description.
	ErrInvalidAction = errors.New("planning: invalid action")
	// ErrInvalidActionCost reports a panicking, failing, negative, or non-finite
	// Action cost evaluation.
	ErrInvalidActionCost = errors.New("planning: invalid action cost")
	// ErrInvalidPlan reports a malformed Planner result.
	ErrInvalidPlan = errors.New("planning: invalid plan")
	// ErrInvalidProblem reports an inconsistent Planning problem.
	ErrInvalidProblem = errors.New("planning: invalid problem")
)
