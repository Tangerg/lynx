// Package codebase owns the semantic index's durable values.
package codebase

import "time"

type Document struct {
	Path      string    `json:"path"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
	Snippet   string    `json:"snippet"`
	Vector    []float64 `json:"vector"`
}

type Index struct {
	Workspace  string
	State      string
	OperationID string
	ModelID    string
	FileCount  int
	ChunkCount int
	Truncated  bool
	IndexedAt  *time.Time
}
