package chat

import "errors"

// ErrSessionNotFound surfaces when StartTurn / Events targets a
// non-existent session.
var ErrSessionNotFound = errors.New("chat: session not found")

// ErrTurnNotFound surfaces when Events / Cancel / InjectSteering
// targets a non-existent or already-finished turn.
var ErrTurnNotFound = errors.New("chat: turn not found")

// ErrTurnInFlight surfaces when StartTurn races with an already-
// running turn on the same session. MVP allows only one turn per
// session at a time.
var ErrTurnInFlight = errors.New("chat: turn already in flight")
