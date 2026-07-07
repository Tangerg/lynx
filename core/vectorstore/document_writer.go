package vectorstore

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/document"
)

type writeFunc func(ctx context.Context, docs []*document.Document) error

func (w writeFunc) Write(ctx context.Context, docs []*document.Document) error {
	return w(ctx, docs)
}

// NewDocumentWriter wraps a [Creator] as a [document.Writer], so the
// vector store fits into pipelines built from generic document
// reader/writer interfaces.
//
// Example:
//
//	writer := vectorstore.NewDocumentWriter(myVectorStore)
//	err := writer.Write(ctx, documents)
func NewDocumentWriter(creator Creator) document.Writer {
	return writeFunc(func(ctx context.Context, docs []*document.Document) error {
		req, err := NewCreateRequest(docs)
		if err != nil {
			return fmt.Errorf("vectorstore.NewDocumentWriter: %w", err)
		}
		return creator.Create(ctx, req)
	})
}
