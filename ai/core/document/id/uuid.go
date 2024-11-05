package id

import (
	"github.com/google/uuid"
)

var _ Generator = (*UUIDGenerator)(nil)

type UUIDGenerator struct {
}

func (u *UUIDGenerator) GenerateId(_ ...any) string {
	return uuid.New().String()
}
