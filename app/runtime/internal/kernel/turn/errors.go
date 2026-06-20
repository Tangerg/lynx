package turn

import "errors"

// ErrTurnNotFound surfaces when Events / Cancel / InjectSteering
// targets a non-existent or already-finished turn.
var ErrTurnNotFound = errors.New("turn: turn not found")

// ErrParkClaimed surfaces from Resume when the turn still exists but its parked
// interrupt was already claimed by a concurrent Cancel (which is driving it to a
// canceled terminal). Distinct from ErrTurnNotFound — which means the live turn
// is gone (a restart), the only case Resume should rebuild from a snapshot. The
// caller must NOT rehydrate on ErrParkClaimed: doing so would resurrect a turn
// the user just canceled (and fire its pending tool). Cancel wins this race.
var ErrParkClaimed = errors.New("turn: park already claimed")
