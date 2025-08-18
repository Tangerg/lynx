package idgenerators

import (
	"context"

	"github.com/google/uuid"
)

var _ IDGenerator = (*UUIDGenerator)(nil)

type UUIDGenerator struct{}

func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

func (u *UUIDGenerator) Generate(_ context.Context, _ ...any) (string, error) {
	return uuid.New().String(), nil
}
