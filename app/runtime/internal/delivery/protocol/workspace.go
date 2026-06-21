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
	WorkspaceListFiles(ctx context.Context, in ListFilesRequest) (*Page[FileEntry], error)
	WorkspaceReadFile(ctx context.Context, in ReadFileRequest) (*FileContent, error)
	WorkspaceListProjects(ctx context.Context, q PageQuery) (*Page[Project], error)
	WorkspaceListSkills(ctx context.Context, in WorkspaceListQuery) (*Page[Skill], error)
	WorkspaceListAgentDocs(ctx context.Context, in WorkspaceListQuery) (*Page[AgentDoc], error)
	WorkspaceMCPListServers(ctx context.Context, q PageQuery) (*Page[McpServer], error)
	WorkspaceMCPListTools(ctx context.Context, in MCPListToolsRequest) (*Page[McpTool], error)
	WorkspaceMCPReconnect(ctx context.Context, server string) error
	WorkspaceMCPAuthorize(ctx context.Context, server string) error
	// MCP-server registry CRUD (the editable configuration the settings pane
	// drives, distinct from the read-only listServers status).
	WorkspaceMCPListConfigs(ctx context.Context, q PageQuery) (*Page[McpServerConfig], error)
	WorkspaceMCPConfigure(ctx context.Context, in ConfigureMCPServerRequest) (*McpServerConfig, error)
	WorkspaceMCPRemove(ctx context.Context, name string) error
	WorkspaceMCPSetEnabled(ctx context.Context, in SetMCPEnabledRequest) error
	WorkspaceMCPTest(ctx context.Context, in ConfigureMCPServerRequest) (*McpTestResult, error)
	// Lifecycle-hooks management (workspace.hooks.*): list the discovered hooks
	// for a cwd (global + project, with trust status) and toggle project trust.
	WorkspaceListHooks(ctx context.Context, in ListHooksRequest) (*HooksListResult, error)
	WorkspaceSetHookTrust(ctx context.Context, in SetHookTrustRequest) error
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

// WorkspaceEventType discriminates the WorkspaceEvent union (AUX_API §3.2).
type WorkspaceEventType string

const (
	WorkspaceEventFilesChanged     WorkspaceEventType = "files.changed"
	WorkspaceEventSkillsChanged    WorkspaceEventType = "skills.changed"
	WorkspaceEventMCPServerChanged WorkspaceEventType = "mcp.serverChanged"
	WorkspaceEventResync           WorkspaceEventType = "resync"
)

// WorkspaceEvent is one non-run workspace event (AUX_API §3.2) — a flat
// tag-discriminated struct (single `type`, optional fields per tag, §2.1).
// Types: files.changed | skills.changed | mcp.serverChanged | resync.
type WorkspaceEvent struct {
	Type WorkspaceEventType `json:"type"`
	// files.changed
	WatchID string   `json:"watchId,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	// Cwd scopes a tool-derived files.changed to the session's working
	// directory (paths are relative to it) — set when the change comes from an
	// agent file tool rather than a client-registered watch, so a client can
	// tell whether the change belongs to the project it's showing.
	Cwd string `json:"cwd,omitempty"`
	// mcp.serverChanged
	Server    string       `json:"server,omitempty"`
	Status    McpStatus    `json:"status,omitempty"`
	ToolCount *int         `json:"toolCount,omitempty"`
	Error     *ProblemData `json:"error,omitempty"`
}

// WorkspaceQuery is the common cwd input for workspace reads (API.md §7.5).
type WorkspaceQuery struct {
	Cwd string `json:"cwd,omitempty"`
}

// DiffMode selects the baseline workspace.getDiff compares against (AUX_API §2.3).
type DiffMode string

const (
	DiffModeWorktree DiffMode = "worktree" // changes vs HEAD, incl. untracked (default)
	DiffModeBase     DiffMode = "base"     // vs merge-base with the default branch
)

// DiffFormat selects the workspace.getDiff result shape (AUX_API §2.3).
type DiffFormat string

const (
	DiffFormatRows DiffFormat = "rows" // per-file structured diff (default)
	DiffFormatRaw  DiffFormat = "raw"  // unified patch string
)

// GetDiffRequest — workspace.getDiff body (AUX_API §2.3). Mode selects the
// baseline (worktree=changes vs HEAD incl. untracked; base=vs merge-base with
// default branch). Format selects the shape (rows=structured; raw=unified
// patch string). Limit caps the diff rows (rows format); over it the result is
// truncated at a file boundary (Diff.Truncated) rather than silently dropped.
type GetDiffRequest struct {
	Cwd    string     `json:"cwd,omitempty"`
	Path   string     `json:"path,omitempty"`
	Mode   DiffMode   `json:"mode,omitempty"`   // "worktree" (default) | "base"
	Format DiffFormat `json:"format,omitempty"` // "rows" (default) | "raw"
	Limit  int        `json:"limit,omitempty"`
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

// ListFilesRequest — workspace.listFiles body (API.md §7.5). Lists files under
// Path (relative to Cwd, jailed). Recursive (or a Glob) yields a flat subtree
// file list — the @file / fuzzy source; otherwise the immediate children — the
// lazy file-tree level. .gitignore + backstop excludes apply unless
// IncludeIgnored. PageQuery carries the limit (cursor unused — bounded list).
type ListFilesRequest struct {
	Cwd            string `json:"cwd,omitempty"`
	Path           string `json:"path,omitempty"`
	Glob           string `json:"glob,omitempty"`
	Recursive      bool   `json:"recursive,omitempty"`
	IncludeIgnored bool   `json:"includeIgnored,omitempty"`
	PageQuery
}

// ReadFileRequest — workspace.readFile body (API.md §7.5). Reads the whole
// file, or the StartLine..EndLine window (1-based inclusive, editor-facing)
// when given. MaxBytes caps an over-large read (the executor self-describes the
// cut via FileContent.Truncated).
type ReadFileRequest struct {
	Cwd       string `json:"cwd,omitempty"`
	Path      string `json:"path"`
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	MaxBytes  int    `json:"maxBytes,omitempty"`
}

// FileContent is the workspace.readFile result (API.md §7.5). TotalLines is the
// whole-file line count even for a windowed read (so the UI can show "12–40 /
// 320"). StartLine/EndLine echo the served window (1-based inclusive), set only
// when a range was requested.
type FileContent struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"` // always "utf-8" (binary files error)
	TotalLines int    `json:"totalLines"`
	Truncated  bool   `json:"truncated,omitempty"`
	StartLine  int    `json:"startLine,omitempty"`
	EndLine    int    `json:"endLine,omitempty"`
}

// FileEntryType is a listed entry's kind (workspace.listFiles, API.md §7.5).
type FileEntryType string

const (
	FileEntryFile    FileEntryType = "file"
	FileEntryDir     FileEntryType = "dir"
	FileEntrySymlink FileEntryType = "symlink"
)

// FileEntry is one entry in workspace.listFiles (API.md §7.5). Path is relative
// to the workspace root. SizeBytes/ModifiedAt are optional and currently
// unpopulated — the consumers (file tree + @file) don't need them and statting
// every entry of a recursive list would dominate the call.
type FileEntry struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Type       FileEntryType `json:"type"`
	SizeBytes  int64         `json:"sizeBytes,omitempty"`
	ModifiedAt string        `json:"modifiedAt,omitempty"`
}

// MCPListToolsRequest — workspace.mcp.listTools body.
type MCPListToolsRequest struct {
	Server string `json:"server,omitempty"`
	PageQuery
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

// FileStatus is the past-tense working-tree status vocabulary shared by
// WorkspaceFileChange, FileDiff, and FileEdit (§4.5). "untracked" is VCS-only
// (a tool's FileEdit never produces it).
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

// FileDiff is one file's structured diff (AUX_API §2.3). Added/Removed are
// omitted for a Binary file (Rows empty) rather than reported as a fake 0;
// PreviousPath is set only for renames.
type FileDiff struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"` // see FileStatus
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitempty"`
	Removed      *int       `json:"removed,omitempty"`
	Binary       bool       `json:"binary,omitempty"`
	Rows         []DiffRow  `json:"rows"`
}

// WorkspaceFileChange is one entry in workspace.listFileChanges (AUX_API §2.2)
// — the VCS working-tree scan state. Distinct from FileEdit (a tool's edit
// result): this one has "untracked" (a VCS-only state); they share the
// past-tense status vocabulary deliberately (§4.5). Added/Removed are omitted
// for a Binary file (not a fake 0); PreviousPath is set only for renames.
type WorkspaceFileChange struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"` // see FileStatus
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitempty"`
	Removed      *int       `json:"removed,omitempty"`
	Binary       bool       `json:"binary,omitempty"`
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

// SkillSource is where a discovered Skill came from (API.md §4.10):
// project (<cwd>/.lyra/skills) or global (<LYRA_HOME>/skills).
type SkillSource string

const (
	SkillSourceProject SkillSource = "project"
	SkillSourceGlobal  SkillSource = "global"
)

// Skill is one entry in workspace.listSkills (API.md §4.10).
type Skill struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Source      SkillSource `json:"source,omitempty"` // see SkillSource
}

// AgentDocScope is where an AGENTS.md was discovered in the cwd→home hierarchy
// (API.md §4.10). Mirrors MemoryScope's values but is a distinct domain (left
// separate rather than DRY-coupled — two scopes is under the rule-of-three).
type AgentDocScope string

const (
	AgentDocScopeCwd         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
	AgentDocScopeHome        AgentDocScope = "home"
)

// AgentDoc is one AGENTS.md discovered from cwd upward (API.md §4.10).
type AgentDoc struct {
	Path  string        `json:"path"`
	Title string        `json:"title,omitempty"`
	Scope AgentDocScope `json:"scope"` // see AgentDocScope
}

// McpStatus is an MCP server's connection state (AUX_API §5.1). Carried on
// McpServer.Status and the mcp.serverChanged WorkspaceEvent.
type McpStatus string

const (
	McpConnecting   McpStatus = "connecting"
	McpConnected    McpStatus = "connected"
	McpDisconnected McpStatus = "disconnected"
	McpFailed       McpStatus = "failed"
	McpNeedsAuth    McpStatus = "needsAuth"
)

// McpAuthStatus is an MCP server's auth posture (AUX_API §5.1); omitted when
// the server tracks no auth.
type McpAuthStatus string

const (
	McpAuthNone        McpAuthStatus = "none"
	McpAuthBearerToken McpAuthStatus = "bearerToken"
	McpAuthOAuth       McpAuthStatus = "oauth"
	McpAuthNotLoggedIn McpAuthStatus = "notLoggedIn"
)

// McpServer is one configured MCP server (API.md §4.10).
type McpServer struct {
	Name       string        `json:"name"`
	Status     McpStatus     `json:"status"` // see McpStatus
	ToolCount  *int          `json:"toolCount,omitempty"`
	AuthStatus McpAuthStatus `json:"authStatus,omitempty"` // see McpAuthStatus; omitted when untracked
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

// McpServerConfig is one entry in the MCP-server registry — the editable
// configuration (workspace.mcp.listConfigs / configure), distinct from McpServer
// (the live status from listServers). The bearer token is returned masked;
// Status / ToolCount / Error are the best-effort live state, present when the
// server is enabled and has been dialed.
type McpServerConfig struct {
	Name                string            `json:"name"`
	Transport           string            `json:"type"` // "stdio" | "streamableHttp" (standard mcpServers vocab)
	Enabled             bool              `json:"enabled"`
	Description         string            `json:"description,omitempty"`
	URL                 string            `json:"url,omitempty"`                 // http transport
	AuthorizationMasked string            `json:"authorizationMasked,omitempty"` // http; "" = none
	Headers             map[string]string `json:"headers,omitempty"`             // http; extra request headers (not masked)
	Command             string            `json:"command,omitempty"`             // stdio transport
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"` // stdio; KEY→value, replaces subprocess env
	Dir                 string            `json:"dir,omitempty"`
	TimeoutSeconds      int               `json:"timeoutSeconds,omitempty"`   // connect-handshake bound; 0 = unbounded
	DisabledTools       []string          `json:"disabledTools,omitempty"`    // hidden from the model
	AutoApproveTools    []string          `json:"autoApproveTools,omitempty"` // skip the approval gate
	Status              McpStatus         `json:"status,omitempty"`           // live, when enabled+dialed
	ToolCount           *int              `json:"toolCount,omitempty"`
	Error               *ProblemData      `json:"error,omitempty"`
}

// ConfigureMCPServerRequest — workspace.mcp.configure / test body (the editable
// fields of McpServerConfig). Authorization is the RAW bearer token (http only);
// an empty Authorization when (re)configuring or testing an EXISTING server
// preserves its stored token, so editing other fields needn't re-enter the
// secret — clear a token by removing the server, not by blanking it.
type ConfigureMCPServerRequest struct {
	Name             string            `json:"name"`
	Transport        string            `json:"type"`
	Enabled          bool              `json:"enabled"`
	Description      string            `json:"description,omitempty"`
	URL              string            `json:"url,omitempty"`
	Authorization    string            `json:"authorization,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Command          string            `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Dir              string            `json:"dir,omitempty"`
	TimeoutSeconds   int               `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string          `json:"disabledTools,omitempty"`
	AutoApproveTools []string          `json:"autoApproveTools,omitempty"`
}

// SetMCPEnabledRequest — workspace.mcp.setEnabled body.
type SetMCPEnabledRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// McpTestResult — workspace.mcp.test result (a connection probe; mirrors
// ProviderTestResult).
type McpTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}
