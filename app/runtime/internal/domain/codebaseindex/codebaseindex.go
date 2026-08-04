// Package codebaseindex models a project's semantic code index. It owns source
// chunks, embeddings, incremental content identities, index lifecycle, and
// similarity-query results. Persistence and search strategy are implementation
// choices outside this package.
package codebaseindex

import (
	"context"
	"errors"
	"time"
)

// ErrNoEmbeddingModel is returned when no embedding model is configured — the
// feature is off. Callers surface it as "configure an embedding-capable
// provider" rather than a hard failure.
var ErrNoEmbeddingModel = errors.New("codebaseindex: no embedding model configured")

// State is an index's lifecycle phase for the status surface.
type State string

const (
	StateNone     State = "none"     // never indexed
	StateIndexing State = "indexing" // a build is in progress
	StateReady    State = "ready"    // searchable
	StateError    State = "error"    // last build failed
)

// Chunk is one indexed code span: a line range of a file plus its embedding.
type Chunk struct {
	Path      string // relative to the project cwd, slash-separated
	StartLine int    // 1-based inclusive
	EndLine   int    // 1-based inclusive
	Text      string
	Embedding []float32
}

// Hit is one search result — a chunk with its similarity score (cosine, [0,1]).
type Hit struct {
	Path      string
	StartLine int
	EndLine   int
	Text      string
	Score     float64
}

// Status is the per-cwd index state. Truncated reports that the project exceeded
// its index caps, so consumers can distinguish a partial index from a complete one.
type Status struct {
	State      State
	ModelID    string
	FileCount  int
	ChunkCount int
	IndexedAt  time.Time
	Truncated  bool
}

// Meta is the persisted per-cwd index header (the model the vectors were built
// with + counts/timestamp). ModelID = "provider:model".
type Meta struct {
	Cwd        string
	ModelID    string
	IndexedAt  time.Time
	FileCount  int
	ChunkCount int
	Truncated  bool
}

// Embedder embeds texts into vectors. The interface lives here (consumer side):
// a caller supplies an implementation over the selected embedding model.
// ID is "provider:model" — it tags the stored vectors so a model change
// invalidates them.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ID() string
}

// Store persists a cwd's semantic index.
type Store interface {
	// Meta returns the cwd's index header; ok=false when never indexed.
	Meta(ctx context.Context, cwd string) (Meta, bool, error)
	// SetMeta upserts the cwd's index header after a build pass.
	SetMeta(ctx context.Context, m Meta) error
	// FileHashes returns path→content-hash for cwd — the incremental diff input.
	FileHashes(ctx context.Context, cwd string) (map[string]string, error)
	// ReplaceFile atomically swaps one file's chunks + hash (delete old, insert
	// new) — the per-file incremental update.
	ReplaceFile(ctx context.Context, cwd, path, hash string, chunks []Chunk) error
	// DeleteFile drops a file's chunks + hash (the file left the project).
	DeleteFile(ctx context.Context, cwd, path string) error
	// AllChunks loads every chunk (with embedding) for cwd — the search corpus.
	AllChunks(ctx context.Context, cwd string) ([]Chunk, error)
	// Clear wipes a cwd's whole index (model changed / forced rebuild).
	Clear(ctx context.Context, cwd string) error
}

// Source reads a cwd's indexable project files and chunks their text.
type Source interface {
	Files(ctx context.Context, cwd string) (files []string, truncated bool, err error)
	Chunks(cwd, path string) (chunks []Chunk, hash string, ok bool)
}
