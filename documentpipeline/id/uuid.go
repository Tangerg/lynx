package id

import (
	"context"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
)

var _ Generator = (*UUIDGenerator)(nil)

// UUIDGenerator returns a fresh random v4 UUID for every document. Use it
// when IDs should be unique even for identical content.
type UUIDGenerator struct{}

func NewUUIDGenerator() *UUIDGenerator { return &UUIDGenerator{} }

func (*UUIDGenerator) Generate(ctx context.Context, doc *document.Document) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if doc == nil {
		return "", ErrNilDocument
	}
	return uuid.New().String(), nil
}
