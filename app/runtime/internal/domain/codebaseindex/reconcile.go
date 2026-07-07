package codebaseindex

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// EnsureIndexed builds or incrementally refreshes cwd's index. No-op on the
// fast path (corpus loaded, same model, scanned within the debounce window).
func (ix *Indexer) EnsureIndexed(ctx context.Context, cwd string) error {
	emb, err := ix.resolve(ctx)
	if err != nil {
		return err
	}
	cwd = worktree.CanonicalCwd(cwd)
	modelID := emb.ID()
	if ix.fresh(cwd, modelID) {
		return nil
	}

	lock := ix.cwdLock(cwd)
	lock.Lock()
	defer lock.Unlock()
	if ix.fresh(cwd, modelID) { // another goroutine may have built it while we waited
		return nil
	}
	return ix.reconcile(ctx, cwd, emb, modelID, false)
}

// Reindex forces a full rebuild of cwd's index from scratch.
func (ix *Indexer) Reindex(ctx context.Context, cwd string) error {
	emb, err := ix.resolve(ctx)
	if err != nil {
		return err
	}
	cwd = worktree.CanonicalCwd(cwd)
	lock := ix.cwdLock(cwd)
	lock.Lock()
	defer lock.Unlock()
	return ix.reconcile(ctx, cwd, emb, emb.ID(), true)
}

// reconcile is the build pass (run under the cwd lock): wipe on model change /
// force, discover files, re-embed only changed/new ones (by content hash), drop
// removed ones, then reload the in-memory corpus + persist meta.
func (ix *Indexer) reconcile(ctx context.Context, cwd string, emb Embedder, modelID string, force bool) error {
	ix.markIndexing(cwd, modelID)

	meta, _, err := ix.store.Meta(ctx, cwd)
	if err != nil {
		return ix.fail(cwd, err)
	}
	if force || (meta.ModelID != "" && meta.ModelID != modelID) {
		if err := ix.store.Clear(ctx, cwd); err != nil {
			return ix.fail(cwd, err)
		}
	}

	files, truncated, err := ix.source.Files(ctx, cwd)
	if err != nil {
		return ix.fail(cwd, err)
	}
	stored, err := ix.store.FileHashes(ctx, cwd)
	if err != nil {
		return ix.fail(cwd, err)
	}

	current := make(map[string]struct{}, len(files))
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return ix.fail(cwd, err)
		}
		current[rel] = struct{}{}
		chunks, hash, ok := ix.source.Chunks(cwd, rel)
		if !ok {
			continue
		}
		if stored[rel] == hash {
			continue // unchanged → keep its vectors
		}
		if err := ix.embedChunks(ctx, emb, chunks); err != nil {
			return ix.fail(cwd, err)
		}
		if err := ix.store.ReplaceFile(ctx, cwd, rel, hash, chunks); err != nil {
			return ix.fail(cwd, err)
		}
	}
	for rel := range stored { // files that left the project → drop their vectors
		if _, still := current[rel]; !still {
			if err := ix.store.DeleteFile(ctx, cwd, rel); err != nil {
				return ix.fail(cwd, err)
			}
		}
	}

	all, err := ix.store.AllChunks(ctx, cwd)
	if err != nil {
		return ix.fail(cwd, err)
	}
	now := time.Now()
	m := Meta{Cwd: cwd, ModelID: modelID, IndexedAt: now, FileCount: len(current), ChunkCount: len(all), Truncated: truncated}
	if err := ix.store.SetMeta(ctx, m); err != nil {
		return ix.fail(cwd, err)
	}

	ix.mu.Lock()
	ix.corpus[cwd] = &loaded{chunks: all, scannedAt: now, modelID: modelID}
	ix.status[cwd] = Status{State: StateReady, ModelID: modelID, FileCount: len(current), ChunkCount: len(all), IndexedAt: now, Truncated: truncated}
	ix.mu.Unlock()
	return nil
}

func (ix *Indexer) markIndexing(cwd, modelID string) {
	ix.mu.Lock()
	prev := ix.status[cwd]
	ix.status[cwd] = Status{State: StateIndexing, ModelID: modelID, FileCount: prev.FileCount, ChunkCount: prev.ChunkCount, IndexedAt: prev.IndexedAt}
	ix.mu.Unlock()
}

func (ix *Indexer) fail(cwd string, err error) error {
	ix.mu.Lock()
	prev := ix.status[cwd]
	ix.status[cwd] = Status{State: StateError, ModelID: prev.ModelID, FileCount: prev.FileCount, ChunkCount: prev.ChunkCount, IndexedAt: prev.IndexedAt, Err: err.Error()}
	ix.mu.Unlock()
	return err
}
