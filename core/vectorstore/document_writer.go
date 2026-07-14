package vectorstore

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

type writeFunc func(ctx context.Context, docs []*document.Document) error

func (w writeFunc) Write(ctx context.Context, docs []*document.Document) error {
	return w(ctx, docs)
}

// NewDocumentWriter wraps an [Indexer] as a [document.Writer], so the
// vector store fits into pipelines built from generic document
// reader/writer interfaces.
//
// Example:
//
//	writer := vectorstore.NewDocumentWriter(myVectorStore)
//	err := writer.Write(ctx, documents)
func NewDocumentWriter(indexer Indexer) document.Writer {
	return writeFunc(func(ctx context.Context, docs []*document.Document) error {
		if len(docs) == 0 {
			return ErrEmptyDocuments
		}
		return indexer.Add(ctx, docs)
	})
}
