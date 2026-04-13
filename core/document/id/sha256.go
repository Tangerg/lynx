package id

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

var _ Generator = (*Sha256Generator)(nil)

type Sha256Generator struct {
	salt []byte
}

func NewSha256Generator(salt []byte) *Sha256Generator {
	return &Sha256Generator{
		salt: salt,
	}
}

func (s *Sha256Generator) Generate(_ context.Context, objects ...any) (string, error) {
	if len(objects) == 0 {
		return "", nil
	}

	hasher := sha256.New()

	for _, obj := range objects {
		if data, err := json.Marshal(obj); err == nil {
			hasher.Write(data)
		}
	}

	hashBytes := hasher.Sum(s.salt)
	return hex.EncodeToString(hashBytes), nil
}
