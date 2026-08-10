// Package codebaseindex defines the stable values and similarity ranking used
// by a project's semantic code index. Corpus discovery, embedding acquisition,
// build lifecycle orchestration, and persistence remain caller concerns.
package codebaseindex

import (
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

// Hit is one search result — a chunk with its cosine similarity score in
// [-1, 1].
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
	CWD        string
	ModelID    string
	IndexedAt  time.Time
	FileCount  int
	ChunkCount int
	Truncated  bool
}
