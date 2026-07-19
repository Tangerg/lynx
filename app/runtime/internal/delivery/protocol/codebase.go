package protocol

import "context"

// Codebase is the codebase.* method group (API.md §7.10) — the @codebase
// semantic index. The agent reaches it through the codebase_search tool; a
// client reaches it here for the @codebase mention (search), the index status
// surface, and a manual reindex. Requires a configured embedding role
// (models.setEmbeddingRole) — otherwise search reports it's unavailable.
type Codebase interface {
	// CodebaseSearch returns the chunks most semantically similar to the query in
	// the cwd's project, building/refreshing the index first.
	CodebaseSearch(ctx context.Context, in CodebaseSearchRequest) (*CodebaseSearchResult, error)
	// CodebaseStatus reports the cwd's index state (for a status surface).
	CodebaseStatus(ctx context.Context, in CodebaseStatusRequest) (*CodebaseStatus, error)
	// CodebaseReindex kicks a full rebuild of the cwd's index in the background
	// (the status surface polls CodebaseStatus for progress). Returns immediately.
	CodebaseReindex(ctx context.Context, in CodebaseReindexRequest) (*CodebaseReindexResponse, error)
}

// CodebaseSearchRequest — codebase.search body. Cwd scopes the project (empty =
// serve dir); Limit caps the hits (default 8).
type CodebaseSearchRequest struct {
	Cwd   string `json:"cwd,omitempty"`
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// CodebaseHit is one search result — a code span with its cosine score [0,1].
type CodebaseHit struct {
	Path      string  `json:"path"`
	StartLine int     `json:"startLine"`
	EndLine   int     `json:"endLine"`
	Snippet   string  `json:"snippet"`
	Score     float64 `json:"score"`
}

// CodebaseSearchResult — the codebase.search reply.
type CodebaseSearchResult struct {
	Hits []CodebaseHit `json:"hits"`
}

// CodebaseStatusRequest / CodebaseReindexRequest — cwd-scoped (empty = serve dir).
type CodebaseStatusRequest struct {
	Cwd string `json:"cwd,omitempty"`
}
type CodebaseReindexRequest struct {
	Cwd string `json:"cwd,omitempty"`
}

// CodebaseState is the index lifecycle phase (CodebaseStatus.state).
type CodebaseState string

const (
	CodebaseStateNone     CodebaseState = "none"     // never indexed
	CodebaseStateIndexing CodebaseState = "indexing" // a build is in progress
	CodebaseStateReady    CodebaseState = "ready"    // searchable
	CodebaseStateError    CodebaseState = "error"    // last build failed (error set)
)

// CodebaseStatus — the codebase.status reply. Truncated reports the project hit
// the index caps (partial index). Error carries the last build failure.
type CodebaseStatus struct {
	State       CodebaseState `json:"state"`
	ModelID     string        `json:"modelId,omitempty"`
	FileCount   int           `json:"fileCount"`
	ChunkCount  int           `json:"chunkCount"`
	IndexedAt   string        `json:"indexedAt,omitempty"`
	Truncated   bool          `json:"truncated,omitempty"`
	Error       string        `json:"error,omitempty"`
	OperationID string        `json:"operationId,omitempty"`
}

type CodebaseReindexResponse struct {
	OperationID string `json:"operationId"`
}
