package etl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
)

// IDGenerator produces an identifier for a document. Implementations may
// derive identity from content or generate an unconditional random identity.
type IDGenerator interface {
	// Generate returns a non-blank identifier for one document without mutating
	// it. Content-addressed implementations must be deterministic; random
	// implementations must still honor ctx and reject nil documents.
	Generate(ctx context.Context, document *document.Document) (string, error)
}

// SHA256IDGenerator builds a content-addressable identifier from a document's
// text, media, and metadata. The existing ID is deliberately excluded so the
// digest remains stable before and after assignment.
type SHA256IDGenerator struct {
	salt []byte
}

func NewSHA256IDGenerator(salt []byte) SHA256IDGenerator {
	return SHA256IDGenerator{salt: bytes.Clone(salt)}
}

// Generate hashes a canonical JSON projection of document and returns the
// SHA-256 hex digest.
func (s SHA256IDGenerator) Generate(ctx context.Context, doc *document.Document) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if doc == nil {
		return "", ErrNilDocument
	}

	hasher := sha256.New()
	if len(s.salt) > 0 {
		_, _ = hasher.Write(s.salt)
		_, _ = hasher.Write([]byte{0})
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
		return "", fmt.Errorf("etl: encode document identity: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type UUIDGenerator struct{}

func (UUIDGenerator) Generate(ctx context.Context, doc *document.Document) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if doc == nil {
		return "", ErrNilDocument
	}
	return uuid.NewString(), nil
}
