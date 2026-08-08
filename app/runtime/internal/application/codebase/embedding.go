package codebase

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// embedChunks fills each chunk's Embedding, batching the embedding calls.
func (ix *Indexer) embedChunks(ctx context.Context, emb Embedder, chunks []codebaseindex.Chunk) error {
	for i := 0; i < len(chunks); i += embedBatch {
		end := min(i+embedBatch, len(chunks))
		texts := make([]string, 0, end-i)
		for _, c := range chunks[i:end] {
			texts = append(texts, c.Text)
		}
		vecs, err := emb.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("codebaseindex: embed batch: %w", err)
		}
		if len(vecs) != end-i {
			return fmt.Errorf("codebaseindex: embed returned %d vectors for %d texts", len(vecs), end-i)
		}
		for j := range vecs {
			chunks[i+j].Embedding = vecs[j]
		}
	}
	return nil
}
