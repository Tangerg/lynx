package id

import (
	"github.com/google/uuid"
)

var _ Generator = (*UUIDGenerator)(nil)

// UUIDGenerator implements the Generator interface using UUID v4.
// It generates random, unique identifiers using the Google UUID library.
type UUIDGenerator struct {
	// No fields required as UUID generation doesn't need state
}

// NewUUIDGenerator creates a new UUIDGenerator instance.
//
// Returns:
//
//	A new UUIDGenerator instance
func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

// GenerateId creates a random UUID string.
// This implementation ignores any input objects and always returns a new
// random UUID in its string representation.
//
// Parameters:
//
//	_ - Objects are ignored by this implementation
//
// Returns:
//
//	A string representation of a randomly generated UUID v4
func (u *UUIDGenerator) GenerateId(_ ...any) string {
	return uuid.New().String()
}
