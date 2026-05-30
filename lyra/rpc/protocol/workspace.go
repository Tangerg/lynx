package protocol

import "context"

// Workspace is the workspace.* method group — the surface backing
// the "what's the project doing" sidebar UI (files-changed, diff,
// grep, terminal output, MCP status). Most of these are stubbed
// until the wider Lyra workspace integration lands.
type Workspace interface {
	WorkspaceFilesChanged(ctx context.Context) ([]FileChange, error)
	WorkspaceDiff(ctx context.Context, path string) ([]DiffRow, error)
	WorkspaceFileHead(ctx context.Context, path string) ([]FileLine, error)
	WorkspaceGrep(ctx context.Context, query string) (*GrepResult, error)

	// WorkspaceTerminalSubscribe streams pty output for a given run's
	// tool terminal. Returns a channel that closes when the run ends
	// or the caller's context cancels.
	WorkspaceTerminalSubscribe(ctx context.Context, runID string) (<-chan TermLine, error)

	WorkspaceProjects(ctx context.Context) ([]Project, error)
	WorkspaceMCPList(ctx context.Context) ([]MCPServer, error)
	WorkspaceMCPReconnect(ctx context.Context, name string) error
	WorkspaceSkills(ctx context.Context) ([]Skill, error)
}

// FileChange is one entry in workspace.filesChanged (API.md §6.5).
type FileChange struct {
	Path    string `json:"path"`
	Change  string `json:"change"` // "add" | "mod" | "del"
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// DiffRow is one structured row of a unified diff. Server-side parser
// turns `git diff` output into rows so the client doesn't regex on
// every render (API.md §6.5).
//
// Discriminator: Type
//
//	"hunk"  → Text   (the `@@ -1,3 +1,3 @@` header)
//	"ctx"   → L, R, Code   (unchanged line, both line numbers)
//	"add"   → R, Code      (added line, new line number)
//	"del"   → L, Code      (removed line, old line number)
type DiffRow struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	L    int    `json:"l,omitempty"`
	R    int    `json:"r,omitempty"`
	Code string `json:"code,omitempty"`
}

// FileLine is one row of a structured file preview (API.md §6.5).
type FileLine struct {
	Ln    string `json:"ln"`   // line number or marker (e.g. "···")
	Code  string `json:"code"` // pre-rendered HTML / highlighted text
	Muted bool   `json:"muted,omitempty"`
}

// GrepMatch is one entry inside GrepResult.Matches.
type GrepMatch struct {
	Path  string `json:"path"`
	Match string `json:"match"`
}

// GrepResult is the workspace.grep result — a single object (NOT
// Page<T>). Total may exceed len(Matches); a future nextCursor field
// would be additive (API.md §6.5).
type GrepResult struct {
	Matches []GrepMatch `json:"matches"`
	Total   int         `json:"total"`
}

// TermLineKind enumerates how a terminal line should be styled.
type TermLineKind string

const (
	TermLineKindPrompt TermLineKind = "prompt"
	TermLineKindCmd    TermLineKind = "cmd"
	TermLineKindOut    TermLineKind = "out"
	TermLineKindErr    TermLineKind = "err"
	TermLineKindWarn   TermLineKind = "warn"
	TermLineKindMute   TermLineKind = "mute"
	TermLineKindOK     TermLineKind = "ok"
)

// TermLine is one line of pty output streamed over
// workspace.terminal.subscribe / notifications/terminal/output
// (API.md §6.5).
type TermLine struct {
	Kind TermLineKind `json:"kind"`
	Text string       `json:"text"`
}

// Project is one entry in workspace.projects (API.md §6.5).
type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Active bool   `json:"active,omitempty"`
}

// MCPServer is one configured MCP server (API.md v4 §6.5).
//
// v4 greenfield cut: this shape used to have `id`, `displayName`, and
// `icon`. All three were dropped:
//   - `id` redundant with MCP-native `name`
//   - `displayName` redundant because MCP server names ("filesystem",
//     "github", "browser") are already human-readable
//   - `icon` is UI presentation that doesn't belong on the wire
type MCPServer struct {
	Name      string `json:"name"` // MCP-native server identifier
	Desc      string `json:"desc"`
	ToolCount int    `json:"toolCount"` // tool count; per-tool detail via workspace.mcp.tools
	Status    string `json:"status"`    // "active" | "idle" | "error"
}

// Skill is one entry in workspace.skills (API.md §6.5).
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
