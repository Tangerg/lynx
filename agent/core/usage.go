package core

import (
	"errors"
	"fmt"
	"math"
)

// ErrInvalidUsage identifies malformed execution-resource usage.
var ErrInvalidUsage = errors.New("usage: invalid")

// Usage is one resource-consumption delta, or an aggregate of such deltas.
// Cost is an opaque non-negative unit chosen consistently by the host; Tokens
// and ModelCalls are framework execution counters.
type Usage struct {
	Cost       float64 `json:"cost"`
	Tokens     int     `json:"tokens"`
	ModelCalls int     `json:"model_calls"`
}

// Validate checks that every usage dimension is finite and non-negative.
func (u Usage) Validate() error {
	if math.IsNaN(u.Cost) || math.IsInf(u.Cost, 0) || u.Cost < 0 {
		return fmt.Errorf("%w: cost must be finite and non-negative", ErrInvalidUsage)
	}
	if u.Tokens < 0 {
		return fmt.Errorf("%w: tokens must not be negative", ErrInvalidUsage)
	}
	if u.ModelCalls < 0 {
		return fmt.Errorf("%w: model calls must not be negative", ErrInvalidUsage)
	}
	return nil
}

// ProcessUsage is the subtree aggregate visible to policies and callers.
// Actions is runtime-owned and derived from completed action history; the
// remaining dimensions are the sum of recorded [Usage] values.
type ProcessUsage struct {
	Usage
	Actions int
}
