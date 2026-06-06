package protocol

import "context"

// Workspace is the workspace.* method group (API.md §7.5). All read
// methods take an optional cwd (default = serve directory); MCP methods
// are runtime-global and take no cwd.
type Workspace interface {
	WorkspaceListFileChanges(ctx context.Context, in WorkspaceQuery) ([]WorkspaceFileChange, error)
	WorkspaceGetDiff(ctx context.Context, in GetDiffRequest) ([]DiffRow, error)
	WorkspaceGetFileHead(ctx context.Context, in GetFileHeadRequest) (*FileHead, error)
	WorkspaceGrep(ctx context.Context, in GrepRequest) (*GrepResult, error)
	WorkspaceListProjects(ctx context.Context) ([]Project, error)
	WorkspaceListSkills(ctx context.Context, in WorkspaceQuery) ([]Skill, error)
	WorkspaceListAgentDocs(ctx context.Context, in WorkspaceQuery) ([]AgentDoc, error)
	WorkspaceMCPListServers(ctx context.Context) ([]McpServer, error)
	WorkspaceMCPListTools(ctx context.Context, in MCPListToolsRequest) ([]McpTool, error)
	WorkspaceMCPReconnect(ctx context.Context, server string) error
}

// WorkspaceQuery is the common cwd input for workspace reads (API.md §7.5).
type WorkspaceQuery struct {
	Cwd string `json:"cwd,omitempty"`
}

// GetDiffRequest — workspace.getDiff body.
type GetDiffRequest struct {
	Cwd  string `json:"cwd,omitempty"`
	Path string `json:"path,omitempty"`
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
}

// WorkspaceFileChange is one entry in workspace.listFileChanges (API.md
// §4.5) — the VCS working-tree scan state. Distinct from FileEdit (a tool's
// edit result): this one has "untracked" (a VCS-only state) and no diff;
// they share the past-tense status vocabulary deliberately (§4.5).
type WorkspaceFileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // "added"|"modified"|"deleted"|"renamed"|"untracked"
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
	Status      string `json:"status"` // "connected" | "disconnected" | "error"
	Description string `json:"description,omitempty"`
}

// McpTool is one tool exposed by an MCP server (API.md §4.10).
type McpTool struct {
	Server      string         `json:"server"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}
