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
	// Self-authored skill-library management (workspace.skills.*): list the
	// global library with each skill's lifecycle, and archive/restore one
	// (never deleting).
	WorkspaceListManagedSkills(ctx context.Context, q PageQuery) (*Page[ManagedSkill], error)
	WorkspaceArchiveSkill(ctx context.Context, in SkillNameRequest) error
	WorkspaceRestoreSkill(ctx context.Context, in SkillNameRequest) error
	WorkspaceListRecipes(ctx context.Context, in WorkspaceListQuery) (*Page[Recipe], error)
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
