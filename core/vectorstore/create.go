package vectorstore

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// CreateRequest is the input to [Creator.Create]: the documents to
// embed, index, and store.
type CreateRequest struct {
	Documents []*document.Document `json:"documents,omitzero"`
}

func NewCreateRequest(docs []*document.Document) (*CreateRequest, error) {
	req := &CreateRequest{Documents: docs}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return req, nil
}

func (r *CreateRequest) Validate() error {
	if r == nil {
		return ErrNilRequest
	}
	if len(r.Documents) == 0 {
		return ErrEmptyDocuments
	}
	return nil
}

// Creator embeds and indexes documents in the vector store. The store
// runs:
//
//  1. Embedding (text → vector)
//  2. Indexing (vector + metadata → searchable record)
//  3. Storage (record → durable backend)
type Creator interface {
	// Create persists the documents in the request.
	Create(ctx context.Context, request *CreateRequest) error
}
