package turn

import "errors"

// ErrTurnNotFound surfaces when Events / Cancel / InjectSteering
// targets a non-existent or already-finished turn.
var ErrTurnNotFound = errors.New("turn: turn not found")
