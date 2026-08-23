package protocol

// CodebaseSearchRequest — codebase.search body. Workspace scopes the project;
// Limit caps the hits (default 8).
type CodebaseSearchRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Query     string       `json:"query"`
	Limit     int          `json:"limit,omitempty"`
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

// CodebaseStatusRequest / CodebaseReindexRequest are explicitly workspace-scoped.
type CodebaseStatusRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
}
type CodebaseReindexRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
}

// CodebaseState is the index lifecycle phase (CodebaseStatus.state).
type CodebaseState string

const (
	CodebaseStateNone     CodebaseState = "none"     // never indexed
	CodebaseStateIndexing CodebaseState = "indexing" // a build is in progress
	CodebaseStateReady    CodebaseState = "ready"    // searchable
	CodebaseStateError    CodebaseState = "error"    // last build failed
)

// CodebaseStatus — the codebase.status reply. Truncated reports the project hit
// the index caps (partial index). Failed rebuilds are represented by StateError;
// implementation diagnostics stay in runtime observability rather than the API.
type CodebaseStatus struct {
	State      CodebaseState `json:"state"`
	ModelID    string        `json:"modelId,omitempty"`
	FileCount  int           `json:"fileCount"`
	ChunkCount int           `json:"chunkCount"`
	IndexedAt  string        `json:"indexedAt,omitempty"`
	Truncated  bool          `json:"truncated,omitempty"`
	// OperationID is present only while State is indexing.
	OperationID string `json:"operationId,omitempty"`
}

type CodebaseReindexResponse struct {
	OperationID string `json:"operationId"`
}
