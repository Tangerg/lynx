package codebaseindex

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var errNilEmbedder = errors.New("codebaseindex: embedding resolver returned a nil embedder")

// Available reports whether an embedding model is configured. A missing model
// is a normal false result; resolver and provider failures remain errors so a
// caller never mistakes an unhealthy dependency for an unconfigured feature.
func (ix *Indexer) Available(ctx context.Context) (bool, error) {
	_, err := ix.resolveEmbedder(ctx)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNoEmbeddingModel):
		return false, nil
	default:
		return false, err
	}
}

func (ix *Indexer) resolveEmbedder(ctx context.Context) (Embedder, error) {
	if ix == nil || ix.resolve == nil {
		return nil, ErrNoEmbeddingModel
	}
	emb, err := ix.resolve(ctx)
	if err != nil {
		return nil, err
	}
	if emb == nil {
		return nil, errNilEmbedder
	}
	return emb, nil
}

// Search embeds the query and returns the topK most-similar chunks, building or
// refreshing the index first.
func (ix *Indexer) Search(ctx context.Context, cwd, query string, topK int) ([]Hit, error) {
	if topK <= 0 {
		topK = defaultTopK
	}
	emb, err := ix.resolveEmbedder(ctx)
	if err != nil {
		return nil, err
	}
	chunks, err := ix.corpusFor(ctx, cwd, emb, emb.ID())
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	vecs, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("codebaseindex: embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, nil
	}

	return topKHits(vecs[0], chunks, topK), nil
}

// Status reports cwd's current index state — the live in-memory status, falling
// back to the persisted meta on a cold process.
func (ix *Indexer) Status(ctx context.Context, cwd string) (Status, error) {
	ix.mu.Lock()
	s, ok := ix.status[cwd]
	ix.mu.Unlock()
	if ok {
		return s, nil
	}
	meta, found, err := ix.store.Meta(ctx, cwd)
	if err != nil {
		return Status{}, err
	}
	if !found {
		return Status{State: StateNone}, nil
	}
	return Status{
		State:      StateReady,
		ModelID:    meta.ModelID,
		FileCount:  meta.FileCount,
		ChunkCount: meta.ChunkCount,
		IndexedAt:  meta.IndexedAt,
		Truncated:  meta.Truncated,
	}, nil
}

// fresh reports whether cwd's corpus is loaded for modelID and scanned within
// the debounce window.
func (ix *Indexer) fresh(cwd, modelID string) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	c := ix.corpus[cwd]
	return c != nil && c.modelID == modelID && time.Since(c.scannedAt) < rescanDebounce
}
