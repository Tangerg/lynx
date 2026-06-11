package protocol

import "context"

// Workspace is the workspace.* method group (API.md §7.5). All read
// methods take an optional cwd (default = serve directory); MCP methods
// are runtime-global and take no cwd.
type Workspace interface {
	WorkspaceListFileChanges(ctx context.Context, in WorkspaceListQuery) (*Page[WorkspaceFileChange], error)
	WorkspaceGetDiff(ctx context.Context, in GetDiffRequest) (*Diff, error)
	WorkspaceGetFileHead(ctx context.Context, in GetFileHeadRequest) (*FileHead, error)
	WorkspaceGrep(ctx context.Context, in GrepRequest) (*GrepResult, error)
	WorkspaceListProjects(ctx context.Context, q PageQuery) (*Page[Project], error)
	WorkspaceListSkills(ctx context.Context, in WorkspaceListQuery) (*Page[Skill], error)
	WorkspaceListAgentDocs(ctx context.Context, in WorkspaceListQuery) (*Page[AgentDoc], error)
	WorkspaceMCPListServers(ctx context.Context, q PageQuery) (*Page[McpServer], error)
	WorkspaceMCPListTools(ctx context.Context, in MCPListToolsRequest) (*Page[McpTool], error)
	WorkspaceMCPReconnect(ctx context.Context, server string) error
	// WorkspaceSubscribe opens the non-run workspace event stream (AUX_API §3):
	// files/skills/mcp changes. Returns an ack + the event channel, closed when
	// the request ctx ends. Streaming method (in streamingMethods).
	WorkspaceSubscribe(ctx context.Context, in WorkspaceSubscribeRequest) (*WorkspaceSubscribeResponse, <-chan WorkspaceEvent, error)
}

// WorkspaceSubscribeRequest — workspace.subscribe body (AUX_API §3.1). Watches
// registers file-monitoring interest; gated behind features.fileWatch.
type WorkspaceSubscribeRequest struct {
	Watches []WatchSpec `json:"watches,omitempty"`
}

// WatchSpec is one file-watch registration. WatchId is client-chosen (echoed
// in files.changed); Cwd defaults to the serve directory; Path is relative to
// Cwd (jailed like §7.5).
type WatchSpec struct {
	WatchID string `json:"watchId"`
	Cwd     string `json:"cwd,omitempty"`
	Path    string `json:"path"`
}

// WorkspaceSubscribeResponse is the (empty) streaming ack — the first frame of
// the stream, mirroring StartRunResponse's role for runs.
type WorkspaceSubscribeResponse struct{}

// WorkspaceEvent is one non-run workspace event (AUX_API §3.2) — a flat
// tag-discriminated struct (single `type`, optional fields per tag, §2.1).
// Types: files.changed | skills.changed | mcp.serverChanged | resync.
type WorkspaceEvent struct {
	Type string `json:"type"`
	// files.changed
	WatchID string   `json:"watchId,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	// mcp.serverChanged
	Server    string       `json:"server,omitempty"`
	Status    string       `json:"status,omitempty"`
	ToolCount *int         `json:"toolCount,omitempty"`
	Error     *ProblemData `json:"error,omitempty"`
}

// WorkspaceQuery is the common cwd input for workspace reads (API.md §7.5).
type WorkspaceQuery struct {
	Cwd string `json:"cwd,omitempty"`
}

// GetDiffRequest — workspace.getDiff body (AUX_API §2.3). Mode selects the
// baseline (worktree=changes vs HEAD incl. untracked; base=vs merge-base with
// default branch). Format selects the shape (rows=structured; raw=unified
// patch string). Limit caps the diff rows (rows format); over it the result is
// truncated at a file boundary (Diff.Truncated) rather than silently dropped.
type GetDiffRequest struct {
	Cwd    string `json:"cwd,omitempty"`
	Path   string `json:"path,omitempty"`
	Mode   string `json:"mode,omitempty"`   // "worktree" (default) | "base"
	Format string `json:"format,omitempty"` // "rows" (default) | "raw"
	Limit  int    `json:"limit,omitempty"`
}

// GetFileHeadRequest — workspace.getFileHead body.
type GetFileHeadRequest struct {
	Cwd   string `json:"cwd,omitempty"`
	Path  string `json:"path"`
	Lines int    `json:"lines,omitempty"`
}

// GrepRequest — workspace.grep body.
type GrepRequest struct {
	Cwd   string `json:"cwd,omitempty"`
	Query string `json:"query"`
	Path  string `json:"path,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// MCPListToolsRequest — workspace.mcp.listTools body.
type MCPListToolsRequest struct {
	Server string `json:"server,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Diff is the workspace.getDiff result (AUX_API §2.3) — a sum type: Files is
// populated for format=rows (per-file structured diff), Patch for format=raw
// (the unified patch string). Truncated self-describes a row-limit cut at a
// file boundary ("no silent caps", §7.5).
type Diff struct {
	Files     []FileDiff `json:"files,omitempty"`
	Patch     string     `json:"patch,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
}

// FileDiff is one file's structured diff (AUX_API §2.3). Added/Removed are
// omitted for a Binary file (Rows empty) rather than reported as a fake 0;
// PreviousPath is set only for renames.
type FileDiff struct {
	Path         string    `json:"path"`
	Status       string    `json:"status"` // "added"|"modified"|"deleted"|"renamed"|"untracked"
	PreviousPath string    `json:"previousPath,omitempty"`
	Added        *int      `json:"added,omitempty"`
	Removed      *int      `json:"removed,omitempty"`
	Binary       bool      `json:"binary,omitempty"`
	Rows         []DiffRow `json:"rows"`
}

// WorkspaceFileChange is one entry in workspace.listFileChanges (AUX_API §2.2)
// — the VCS working-tree scan state. Distinct from FileEdit (a tool's edit
// result): this one has "untracked" (a VCS-only state); they share the
// past-tense status vocabulary deliberately (§4.5). Added/Removed are omitted
// for a Binary file (not a fake 0); PreviousPath is set only for renames.
type WorkspaceFileChange struct {
	Path         string `json:"path"`
	Status       string `json:"status"` // "added"|"modified"|"deleted"|"renamed"|"untracked"
	PreviousPath string `json:"previousPath,omitempty"`
	Added        *int   `json:"added,omitempty"`
	Removed      *int   `json:"removed,omitempty"`
	Binary       bool   `json:"binary,omitempty"`
}

// FileHead is a file preview (API.md §4.5).
type FileHead struct {
	Path  string     `json:"path"`
	Lines []FileLine `json:"lines"`
}

// FileLine is one preview line — plain text, client highlights (API.md §4.5).
type FileLine struct {
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}

// GrepResult is the workspace.grep result (API.md §4.5). Total may
// exceed len(Matches) when limited.
type GrepResult struct {
	Matches []GrepMatch `json:"matches"`
	Total   int         `json:"total"`
}

// GrepMatch is one grep hit — plain text (API.md §4.5).
type GrepMatch struct {
	Path       string `json:"path"`
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}

// Skill is one entry in workspace.listSkills (API.md §4.10).
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

// AgentDoc is one AGENTS.md discovered from cwd upward (API.md §4.10).
type AgentDoc struct {
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
	Scope string `json:"scope"` // "cwd" | "projectRoot" | "home"
}

// McpServer is one configured MCP server (API.md §4.10).
type McpServer struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // AUX_API §5.1: connecting|connected|disconnected|failed|needsAuth
	ToolCount   *int   `json:"toolCount,omitempty"`
	AuthStatus  string `json:"authStatus,omitempty"` // none|bearerToken|oauth|notLoggedIn (omitted when untracked)
	// Error carries the reason for a failed server (AUX_API §5.1); set only
	// when Status is "failed".
	Error       *ProblemData `json:"error,omitempty"`
	Description string       `json:"description,omitempty"`
}

// McpTool is one tool exposed by an MCP server (API.md §4.10).
type McpTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}
