package turn

import "errors"

// ErrTurnNotFound surfaces when Events / Cancel / InjectSteering
// targets a non-existent or already-finished turn.
var ErrTurnNotFound = errors.New("turn: turn not found")

// ErrClosed reports admission after the process-scoped controller
// has begun shutdown.
var ErrClosed = errors.New("turn: controller closed")

// ErrTurnAlreadyActivated reports a duplicate attempt to activate a prepared
// turn. Activation is an ownership transfer and may happen exactly once.
var ErrTurnAlreadyActivated = errors.New("turn: turn already activated")

// ErrShutdownTimeout reports that controller shutdown canceled every turn but
// at least one turn did not finish its terminal teardown within the deadline.
var ErrShutdownTimeout = errors.New("turn: shutdown timeout")

// ErrPromptBlocked surfaces when a UserPromptSubmit / SessionStart hook blocks a
// turn before it starts. The delivery layer maps it to a run-channel error so
// the client sees why the prompt was refused.
var ErrPromptBlocked = errors.New("turn: prompt blocked by a hook")

// ErrParkClaimed surfaces from Resume when the turn still exists but its parked
// interrupt was already claimed by a concurrent Cancel (which is driving it to a
// canceled terminal). Distinct from ErrTurnNotFound — which means the live turn
// is gone (a restart), the only case Resume should rebuild from a snapshot. The
// caller must NOT rehydrate on ErrParkClaimed: doing so would resurrect a turn
// the user just canceled (and fire its pending tool). Cancel wins this race.
var ErrParkClaimed = errors.New("turn: park already claimed")
