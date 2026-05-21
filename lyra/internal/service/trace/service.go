// Package trace defines the TraceService — Lyra's observability surface.
// Every session captures an OTel trace; clients query span trees here
// for debugging and a "what did the agent actually do?" timeline.
package trace

import (
	"context"
	"time"
)

// Trace is the metadata of one captured trace. Trace data itself is
// fetched separately via [Service.GetTrace].
type Trace struct {
	ID         string
	SessionID  string
	TurnID     string
	StartedAt  time.Time
	Duration   time.Duration
	SpanCount  int
}

// Span is one node in a trace's span tree. Children are referenced
// by ID so very deep trees can be paginated on the wire.
type Span struct {
	ID         string
	ParentID   string // empty for root
	Name       string
	StartedAt  time.Time
	Duration   time.Duration
	Attributes map[string]string
}

// Service is the TraceService contract. M7 wires the real
// implementation; M1 leaves it as a stub.
type Service interface {
	// List returns every captured trace, newest first.
	List(ctx context.Context) ([]Trace, error)

	// Get returns the full span tree for the given trace id, or
	// ErrNotFound when missing.
	Get(ctx context.Context, id string) ([]Span, error)

	// StreamLive surfaces spans for a turn as they happen. Useful
	// for "live timeline" UIs. The channel closes when the turn ends.
	StreamLive(ctx context.Context, turnID string) (<-chan Span, error)
}
