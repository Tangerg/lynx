package codebaseindex

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// Available reports whether an embedding model is configured.
func (ix *Indexer) Available(ctx context.Context) bool {
	if ix == nil {
		return false
	}
	emb, err := ix.resolve(ctx)
	return err == nil && emb != nil
}

// Search embeds the query and returns the topK most-similar chunks, building or
// refreshing the index first.
func (ix *Indexer) Search(ctx context.Context, cwd, query string, topK int) ([]Hit, error) {
	if topK <= 0 {
		topK = defaultTopK
	}
	cwd = worktree.CanonicalCwd(cwd)
	if err := ix.EnsureIndexed(ctx, cwd); err != nil {
		return nil, err
	}
	emb, err := ix.resolve(ctx)
	if err != nil {
		return nil, err
	}
	vecs, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("codebaseindex: embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, nil
	}

	ix.mu.Lock()
	c := ix.corpus[cwd]
	ix.mu.Unlock()
	if c == nil || len(c.chunks) == 0 {
		return nil, nil
	}
	return topKHits(vecs[0], c.chunks, topK), nil
}

// Status reports cwd's current index state — the live in-memory status, falling
// back to the persisted meta on a cold process.
func (ix *Indexer) Status(ctx context.Context, cwd string) (Status, error) {
	cwd = worktree.CanonicalCwd(cwd)
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
