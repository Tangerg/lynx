package chat

import "errors"

// ErrSessionNotFound surfaces when StartTurn / Events targets a
// non-existent session.
var ErrSessionNotFound = errors.New("chat: session not found")

// ErrTurnNotFound surfaces when Events / Cancel / InjectSteering
// targets a non-existent or already-finished turn.
var ErrTurnNotFound = errors.New("chat: turn not found")
