package idgenerators

import (
	"context"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/ai/content/document"
)

var _ document.IDGenerator = (*UUIDGenerator)(nil)

type UUIDGenerator struct{}

func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

func (u *UUIDGenerator) Generate(_ context.Context, _ ...any) string {
	return uuid.New().String()
}
