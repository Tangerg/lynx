package id

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

var _ Generator = (*SHA256Generator)(nil)

// SHA256Generator builds a content-addressable identifier by hashing
// the JSON encoding of the supplied objects with SHA-256 and returning
// the hex digest. Identical inputs produce identical ids — useful for
// deduplication across runs.
//
// An optional salt distinguishes hash streams across deployments
// (multi-tenant setups where the same content needs different ids).
type SHA256Generator struct {
	salt []byte
}

// NewSHA256Generator returns a generator with an independent snapshot of salt.
func NewSHA256Generator(salt []byte) *SHA256Generator {
	return &SHA256Generator{salt: bytes.Clone(salt)}
}

// Generate hashes the JSON encoding of each object and returns the hex
// digest. Empty input returns "" with no error. Inputs that fail to
// JSON-marshal (channels / funcs / cyclic refs) propagate the error —
// silent skips would make distinct inputs hash to the same id.
func (s *SHA256Generator) Generate(_ context.Context, objects ...any) (string, error) {
	if len(objects) == 0 {
		return "", nil
	}

	hasher := sha256.New()
	// Mix salt INTO the digest (not appended to its output): hash.Hash.Sum(b)
	// returns b || digest, so calling Sum(salt) would emit salt as a hex
	// prefix while leaving the digest itself unchanged across salts.
	if len(s.salt) > 0 {
		hasher.Write(s.salt)
	}
	for _, obj := range objects {
		data, err := json.Marshal(obj)
		if err != nil {
			return "", fmt.Errorf("id.SHA256Generator: marshal object: %w", err)
		}
		hasher.Write(data)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
