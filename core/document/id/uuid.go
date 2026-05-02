package id

import (
	"context"

	"github.com/google/uuid"
)

var _ Generator = (*UUIDGenerator)(nil)

// UUIDGenerator returns a fresh random v4 UUID on every call,
// ignoring any input objects. Use it when ids should be unique even
// for identical content.
type UUIDGenerator struct{}

// NewUUIDGenerator returns a stateless [UUIDGenerator]. The struct has
// no fields; sharing one across goroutines is fine.
func NewUUIDGenerator() *UUIDGenerator { return &UUIDGenerator{} }

// Generate returns a new v4 UUID. The objects argument is ignored.
func (u *UUIDGenerator) Generate(_ context.Context, _ ...any) (string, error) {
	return uuid.New().String(), nil
}
