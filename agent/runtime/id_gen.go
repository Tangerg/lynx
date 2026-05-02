// Package runtime is the agent execution engine: in-memory blackboard,
// world-state determiner, the AgentProcess state machine, the simple and
// concurrent tick implementations, the executeAction retry loop, and the
// Platform that wires everything together.
package runtime

import (
	"strconv"
	"sync/atomic"

	"github.com/google/uuid"
)

// IDGenerator produces stable, unique IDs for processes. The runtime
// constructs a default UUID-backed generator; tests inject deterministic
// counters.
type IDGenerator interface {
	Next() string
}

// uuidIDGenerator is the production default — UUIDv4.
type uuidIDGenerator struct{}

func (uuidIDGenerator) Next() string { return uuid.NewString() }

// NewUUIDIDGenerator returns the default generator.
func NewUUIDIDGenerator() IDGenerator { return uuidIDGenerator{} }

// CounterIDGenerator is a deterministic generator for tests. Reset by
// constructing a fresh instance.
type CounterIDGenerator struct {
	prefix  string
	counter atomic.Uint64
}

// NewCounterIDGenerator builds a counter-backed generator. prefix shows up
// in the rendered ID ("test-1", "test-2", ...) so test failures are easier
// to debug.
func NewCounterIDGenerator(prefix string) *CounterIDGenerator {
	if prefix == "" {
		prefix = "id"
	}
	return &CounterIDGenerator{prefix: prefix}
}

func (g *CounterIDGenerator) Next() string {
	n := g.counter.Add(1)
	return g.prefix + "-" + strconv.FormatUint(n, 10)
}
