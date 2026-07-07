package codebaseindex

import (
	"context"
	"sync"
	"time"
)

// rescanDebounce bounds how long a freshly-reconciled corpus is trusted before
// the next Search re-diffs the filesystem — collapses rapid successive searches
// in one turn to a single scan while still catching edits between turns.
const rescanDebounce = 5 * time.Second

// defaultTopK is the result count when a caller doesn't specify one.
const defaultTopK = 8

// embedBatch bounds one embedding API call.
const embedBatch = 96

// loaded is a cwd's in-memory search corpus plus when it was last reconciled.
type loaded struct {
	chunks    []Chunk
	scannedAt time.Time
	modelID   string
}

// Indexer is the in-process [Index]: it owns per-cwd build serialization, an
// in-memory corpus cache, and the discover→embed→store→search flow.
type Indexer struct {
	store   Store
	resolve func(context.Context) (Embedder, error) // current embedding model; ErrNoEmbeddingModel when off
	source  Source

	mu     sync.Mutex
	locks  map[string]*sync.Mutex // per-cwd build lock (serializes concurrent builds of one cwd)
	corpus map[string]*loaded     // cwd → in-memory search corpus
	status map[string]Status      // cwd → last known status
}

var _ Index = (*Indexer)(nil)

// New builds an Indexer over the given store, embedding-model resolver, and
// project source. The resolver returns [ErrNoEmbeddingModel] when none is
// configured.
func New(store Store, resolve func(context.Context) (Embedder, error), source Source) *Indexer {
	return &Indexer{
		store:   store,
		resolve: resolve,
		source:  source,
		locks:   map[string]*sync.Mutex{},
		corpus:  map[string]*loaded{},
		status:  map[string]Status{},
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
