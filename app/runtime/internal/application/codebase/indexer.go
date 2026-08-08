package codebase

import (
	"context"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// rescanDebounce bounds how long a freshly-reconciled corpus is trusted before
// the next Search re-diffs the filesystem — collapses rapid successive searches
// in one Run to a single scan while still catching edits between Runs.
const rescanDebounce = 5 * time.Second

// defaultTopK is the result count when a caller doesn't specify one.
const defaultTopK = 8

// embedBatch bounds one embedding API call.
const embedBatch = 96

// loaded is a cwd's in-memory search corpus plus when it was last reconciled.
type loaded struct {
	chunks    []codebaseindex.Chunk
	scannedAt time.Time
	modelID   string
}

// Indexer owns per-cwd build serialization, an
// in-memory corpus cache, and the discover→embed→store→search flow.
type Indexer struct {
	store   Store
	resolve func(context.Context) (Embedder, error) // current embedding model; ErrNoEmbeddingModel when off
	source  Source

	mu     sync.Mutex
	locks  map[string]*sync.Mutex          // per-cwd build lock (serializes concurrent builds of one cwd)
	corpus map[string]*loaded              // cwd → in-memory search corpus
	status map[string]codebaseindex.Status // cwd → last known status
}

// Embedder turns text into vectors and identifies the model that produced them.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ID() string
}

// Store persists one workspace's semantic index.
type Store interface {
	Meta(ctx context.Context, cwd string) (codebaseindex.Meta, bool, error)
	SetMeta(ctx context.Context, meta codebaseindex.Meta) error
	FileHashes(ctx context.Context, cwd string) (map[string]string, error)
	ReplaceFile(ctx context.Context, cwd, path, hash string, chunks []codebaseindex.Chunk) error
	DeleteFile(ctx context.Context, cwd, path string) error
	AllChunks(ctx context.Context, cwd string) ([]codebaseindex.Chunk, error)
	Clear(ctx context.Context, cwd string) error
}

// Source discovers and chunks indexable workspace files.
type Source interface {
	Files(ctx context.Context, cwd string) (files []string, truncated bool, err error)
	Chunks(cwd, path string) (chunks []codebaseindex.Chunk, hash string, ok bool)
}

// NewIndex builds an Indexer over the given store, embedding-model resolver, and
// project source. The resolver returns [codebaseindex.ErrNoEmbeddingModel] when none is
// configured.
func NewIndex(store Store, resolve func(context.Context) (Embedder, error), source Source) *Indexer {
	return &Indexer{
		store:   store,
		resolve: resolve,
		source:  source,
		locks:   map[string]*sync.Mutex{},
		corpus:  map[string]*loaded{},
		status:  map[string]codebaseindex.Status{},
	}
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
