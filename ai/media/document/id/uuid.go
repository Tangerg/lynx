package id

import (
	"context"

	"github.com/google/uuid"
)

var _ Generator = (*UUIDGenerator)(nil)

type UUIDGenerator struct{}

func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

func (u *UUIDGenerator) Generate(_ context.Context, _ ...any) (string, error) {
	return uuid.New().String(), nil
}
