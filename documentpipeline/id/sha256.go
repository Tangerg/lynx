package id

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

var _ Generator = (*SHA256Generator)(nil)

// SHA256Generator builds a content-addressable identifier from a document's
// text, media, and metadata. The existing ID is deliberately excluded so the
// digest remains stable before and after assignment.
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

// Generate hashes a canonical JSON projection of doc and returns the SHA-256
// hex digest.
func (s *SHA256Generator) Generate(ctx context.Context, doc *document.Document) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if doc == nil {
		return "", ErrNilDocument
	}

	hasher := sha256.New()
	if len(s.salt) > 0 {
		hasher.Write(s.salt)
		hasher.Write([]byte{0})
	}
	projection := struct {
		Text     string       `json:"text,omitempty"`
		Media    *media.Media `json:"media,omitempty"`
		Metadata metadata.Map `json:"metadata,omitzero"`
	}{
		Text:     doc.Text,
		Media:    doc.Media,
		Metadata: doc.Metadata,
	}
	if err := json.NewEncoder(hasher).Encode(projection); err != nil {
		return "", fmt.Errorf("document id: encode document: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
