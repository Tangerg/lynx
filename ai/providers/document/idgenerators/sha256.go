package idgenerators

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Tangerg/lynx/ai/content/document"
)

var _ document.IDGenerator = (*Sha256Generator)(nil)

type Sha256Generator struct {
	salt []byte
}

func NewSha256Generator(salt []byte) *Sha256Generator {
	return &Sha256Generator{
		salt: salt,
	}
}

func (s *Sha256Generator) Generate(_ context.Context, objects ...any) string {
	if len(objects) == 0 {
		return ""
	}

	hasher := sha256.New()

	for _, obj := range objects {
		if data, err := json.Marshal(obj); err == nil {
			hasher.Write(data)
		}
	}

	hashBytes := hasher.Sum(s.salt)
	return hex.EncodeToString(hashBytes)
}
