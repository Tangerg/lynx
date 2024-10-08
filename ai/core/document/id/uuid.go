package id

import "github.com/google/uuid"

type UUIDGenerator struct {
}

func (u *UUIDGenerator) GenerateId(_ ...any) string {
	return uuid.New().String()
}
