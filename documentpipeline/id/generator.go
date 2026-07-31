package id

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/document"
)

// ErrNilDocument reports an ID request without a document.
var ErrNilDocument = errors.New("document id: document must not be nil")

// Generator produces an identifier for a document. Implementations may derive
// the ID from document content (see [SHA256Generator]) or generate an
// unconditional random identity (see [UUIDGenerator]).
type Generator interface {
	Generate(ctx context.Context, doc *document.Document) (string, error)
}
