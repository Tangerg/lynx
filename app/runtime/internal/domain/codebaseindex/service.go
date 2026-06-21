package codebaseindex

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"
)

// rescanDebounce bounds how long a freshly-reconciled corpus is trusted before
// the next Search re-diffs the filesystem — collapses rapid successive searches
// in one turn to a single scan while still catching edits between turns.
const rescanDebounce = 5 * time.Second

// defaultTopK is the result count when a caller doesn't specify one.
const defaultTopK = 8

// loaded is a cwd's in-memory search corpus plus when it was last reconciled.
type loaded struct {
	chunks    []Chunk
	scannedAt time.Time
	modelID   string
}

// Indexer is the in-process [Service]: it owns per-cwd build serialization, an
// in-memory corpus cache, and the discover→embed→store→search flow.
type Indexer struct {
	store   Store
	resolve func(context.Context) (Embedder, error) // current embedding model; ErrNoEmbeddingModel when off

	mu     sync.Mutex
	locks  map[string]*sync.Mutex // per-cwd build lock (serializes concurrent builds of one cwd)
	corpus map[string]*loaded     // cwd → in-memory search corpus
	status map[string]Status      // cwd → last known status
}

var _ Service = (*Indexer)(nil)

// New builds an Indexer over the given store and embedding-model resolver. The
// resolver returns [ErrNoEmbeddingModel] when none is configured.
func New(store Store, resolve func(context.Context) (Embedder, error)) *Indexer {
	return &Indexer{
		store:   store,
		resolve: resolve,
		locks:   map[string]*sync.Mutex{},
		corpus:  map[string]*loaded{},
		status:  map[string]Status{},
	}
}

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

// EnsureIndexed builds or incrementally refreshes cwd's index. No-op on the fast
// path (corpus loaded, same model, scanned within the debounce window).
func (ix *Indexer) EnsureIndexed(ctx context.Context, cwd string) error {
	emb, err := ix.resolve(ctx)
	if err != nil {
		return err
	}
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
	lock := ix.cwdLock(cwd)
	lock.Lock()
	defer lock.Unlock()
	return ix.reconcile(ctx, cwd, emb, emb.ID(), true)
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

func (ix *Indexer) cwdLock(cwd string) *sync.Mutex {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	l := ix.locks[cwd]
	if l == nil {
		l = &sync.Mutex{}
		ix.locks[cwd] = l
	}
	return l
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

	files, truncated, err := discoverFiles(ctx, cwd)
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
		chunks, hash, ok := readChunks(cwd, rel)
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

// embedChunks fills each chunk's Embedding, batching the embedding calls.
func (ix *Indexer) embedChunks(ctx context.Context, emb Embedder, chunks []Chunk) error {
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

// topKHits scores every chunk against the query vector by cosine similarity and
// returns the k highest, descending.
func topKHits(query []float32, chunks []Chunk, k int) []Hit {
	qn := norm(query)
	if qn == 0 {
		return nil
	}
	hits := make([]Hit, 0, len(chunks))
	for i := range chunks {
		hits = append(hits, Hit{
			Path:      chunks[i].Path,
			StartLine: chunks[i].StartLine,
			EndLine:   chunks[i].EndLine,
			Text:      chunks[i].Text,
			Score:     cosine(query, qn, chunks[i].Embedding),
		})
	}
	slices.SortFunc(hits, func(a, b Hit) int { return cmp.Compare(b.Score, a.Score) })
	return hits[:min(k, len(hits))]
}

func norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

// cosine returns the cosine similarity of v against query (whose precomputed
// norm is qn). 0 for a dimension mismatch or a zero vector.
func cosine(query []float32, qn float64, v []float32) float64 {
	if len(v) != len(query) {
		return 0
	}
	var dot, vn float64
	for i := range query {
		dot += float64(query[i]) * float64(v[i])
		vn += float64(v[i]) * float64(v[i])
	}
	vn = math.Sqrt(vn)
	if vn == 0 {
		return 0
	}
	return dot / (qn * vn)
}
