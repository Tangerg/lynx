package id

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

var _ Generator = (*Sha256Generator)(nil)

// Sha256Generator implements the Generator interface using SHA-256 hashing.
// It creates deterministic IDs by hashing the JSON representation of objects
// along with an optional salt value.
type Sha256Generator struct {
	salt []byte // Optional salt bytes to include in the hash
}

// NewSha256Generator creates a new Sha256Generator with the provided salt bytes.
//
// Parameters:
//
//	salt - Salt bytes to include in the hash generation
//
// Returns:
//
//	A new Sha256Generator instance
func NewSha256Generator(salt []byte) *Sha256Generator {
	return &Sha256Generator{
		salt: salt,
	}
}

// GenerateId creates a deterministic ID by hashing the JSON representation of
// the provided objects along with the salt bytes.
//
// The method:
// 1. Returns an empty string if no objects are provided
// 2. Creates a new SHA-256 hasher
// 3. Marshals each object to JSON and writes the bytes to the hasher
// 4. Combines the hash with the salt bytes and returns the hex-encoded result
//
// Parameters:
//
//	obj - Objects to generate an ID for
//
// Returns:
//
//	A hex-encoded string representation of the SHA-256 hash
func (s *Sha256Generator) GenerateId(obj ...any) string {
	if len(obj) == 0 {
		return ""
	}
	h := sha256.New()
	for _, item := range obj {
		jsonBytes, err := json.Marshal(item)
		if err == nil {
			h.Write(jsonBytes)
		}
	}
	hashBytes := h.Sum(s.salt)
	return hex.EncodeToString(hashBytes)
}
