// Package state contains shared not-found and conflict vocabulary for the
// session-owned Plan, Goal and Interrupt projections.
package state

import "errors"

var (
	ErrNotFound = errors.New("state: resource not found")
	ErrConflict = errors.New("state: revision conflict")
)
