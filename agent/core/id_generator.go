package core

import "github.com/google/uuid"

// UUIDGeneratorName is the default UUID generator's extension identifier.
const UUIDGeneratorName = "uuid"

// IDGenerator produces non-empty IDs that are unique within an Engine's live
// process registry. It is valid only at engine scope; runtime falls back to a
// built-in UUID generator when Config.Extensions contains none.
type IDGenerator interface {
	Extension

	Next() string
}

// UUIDGenerator generates UUIDv4 process IDs.
type UUIDGenerator struct{ name string }

// NewUUIDGenerator returns the default UUID-v4 generator with the
// supplied extension Name (defaults to "uuid" when blank).
func NewUUIDGenerator(name string) *UUIDGenerator {
	if name == "" {
		name = UUIDGeneratorName
	}
	return &UUIDGenerator{name: name}
}

func (g *UUIDGenerator) Name() string { return g.name }
func (*UUIDGenerator) Next() string   { return uuid.NewString() }
