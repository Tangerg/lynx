package id

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

var _ Generator = (*Sha256Generator)(nil)

// Sha256Generator builds a content-addressable identifier by hashing
// the JSON encoding of the supplied objects with SHA-256 and returning
// the hex digest. Identical inputs produce identical ids — useful for
// deduplication across runs.
//
// An optional salt distinguishes hash streams across deployments
// (multi-tenant setups where the same content needs different ids).
type Sha256Generator struct {
	salt []byte
}

// NewSha256Generator returns a generator that prepends salt to every
// hash. Pass nil for an unsalted generator.
func NewSha256Generator(salt []byte) *Sha256Generator {
	return &Sha256Generator{salt: salt}
}

// Generate hashes the JSON encoding of each object and returns the hex
// digest. Empty input returns "" with no error.
//
// Values that fail to JSON-marshal (channels / funcs / cyclic refs)
// are skipped silently — those are programmer errors callers should
// not encounter in normal use.
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

	return hex.EncodeToString(hasher.Sum(s.salt)), nil
}
