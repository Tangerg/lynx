// Package codebaseindex is the @codebase semantic-index domain: it embeds a
// project's code into vectors and answers similarity queries, so the agent
// (codebase_search tool) and the user (@codebase mention) can find code by
// meaning rather than by literal text.
//
// Storage is sqlite (vectors as float32 BLOBs) + brute-force cosine in Go: a
// single project is a few thousand chunks, so an exact top-k scan is
// microseconds — no external vector server. The index is built lazily on first
// use, persisted across restart, and refreshed incrementally by per-file
// content hash (only changed files are re-embedded). Embeddings from a different
// model aren't comparable, so changing the embedding model invalidates a cwd's
// index (Meta.ModelID guards it).
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
	StateError    State = "error"    // last build failed (Err set)
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

// Status is the per-cwd index state for the management surface
// (codebase.status). Truncated reports that the project exceeded the index caps
// (so the index is partial — "no silent caps").
type Status struct {
	State      State
	ModelID    string
	FileCount  int
	ChunkCount int
	IndexedAt  time.Time
	Truncated  bool
	Err        string
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
// the runtime supplies an implementation over the configured embedding model.
// ID is "provider:model" — it tags the stored vectors so a model change
// invalidates them.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ID() string
}

// Store persists a cwd's index. The sqlite implementation satisfies it.
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

// Index is the semantic-index surface consumed by the tool, RPC, and file-change hook.
type Index interface {
	// Search returns the topK most similar chunks to query in cwd, building or
	// refreshing the index first when needed. ErrNoEmbeddingModel when off.
	Search(ctx context.Context, cwd, query string, topK int) ([]Hit, error)
	// EnsureIndexed builds or incrementally refreshes cwd's index (no-op when
	// fresh). Synchronous — callers run it inline (tool) or in a goroutine (UI).
	EnsureIndexed(ctx context.Context, cwd string) error
	// Reindex forces a full rebuild of cwd's index from scratch.
	Reindex(ctx context.Context, cwd string) error
	// Status reports cwd's current index state for the management surface.
	Status(ctx context.Context, cwd string) (Status, error)
	// Available reports whether an embedding model is configured (the tool is
	// offered only when true).
	Available(ctx context.Context) bool
}
