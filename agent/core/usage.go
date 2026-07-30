package core

import (
	"errors"
	"fmt"
	"math"
)

// ErrInvalidUsage identifies malformed execution-resource usage.
var ErrInvalidUsage = errors.New("usage: invalid")

// Usage is one resource-consumption delta, or an aggregate of such deltas.
// Cost is an opaque non-negative unit chosen consistently by the host. The
// remaining fields are framework execution counters. A ProcessSnapshot stores
// direct usage separately from historical detached-child usage; ProcessView
// reports their aggregate together with live descendants.
type Usage struct {
	Cost       float64 `json:"cost"`
	Tokens     int64   `json:"tokens"`
	ModelCalls int     `json:"model_calls"`
	Actions    int     `json:"actions"`
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
	if u.Actions < 0 {
		return fmt.Errorf("%w: actions must not be negative", ErrInvalidUsage)
	}
	return nil
}
