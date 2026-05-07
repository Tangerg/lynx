package core

import (
	"strconv"
	"sync/atomic"

	"github.com/google/uuid"
)

// IDGenerator produces stable, unique IDs for processes. Doubles as a
// platform-level [Extension] — the runtime registers one through
// PlatformConfig.Extensions and falls back to a built-in UUID generator
// when none is registered.
type IDGenerator interface {
	Extension

	Next() string
}

// uuidIDGenerator is the production default — UUIDv4.
type uuidIDGenerator struct{ name string }

// NewUUIDIDGenerator returns the default UUID-v4 generator with the
// supplied extension Name (defaults to "uuid" when blank).
func NewUUIDIDGenerator(name string) IDGenerator {
	if name == "" {
		name = "uuid"
	}
	return uuidIDGenerator{name: name}
}

func (g uuidIDGenerator) Name() string { return g.name }
func (uuidIDGenerator) Next() string   { return uuid.NewString() }

// CounterIDGenerator is a deterministic generator for tests. Reset by
// constructing a fresh instance.
type CounterIDGenerator struct {
	name    string
	prefix  string
	counter atomic.Uint64
}

// NewCounterIDGenerator builds a counter-backed generator. prefix shows
// up in the rendered ID ("test-1", "test-2", ...) so test failures are
// easier to debug; the extension Name defaults to "counter:"+prefix
// when blank so multiple counters with different prefixes can co-exist
// in the same registration scope.
func NewCounterIDGenerator(prefix string) *CounterIDGenerator {
	if prefix == "" {
		prefix = "id"
	}
	return &CounterIDGenerator{name: "counter:" + prefix, prefix: prefix}
}

func (g *CounterIDGenerator) Name() string { return g.name }

func (g *CounterIDGenerator) Next() string {
	n := g.counter.Add(1)
	return g.prefix + "-" + strconv.FormatUint(n, 10)
}
