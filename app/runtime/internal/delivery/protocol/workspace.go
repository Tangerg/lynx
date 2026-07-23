package protocol

import "context"

// Workspace is the workspace.* method group (API.md §7.5). Its methods stay
// limited to the worktree view: VCS, files, search, projects, and its event
// stream. Independently named wire roots live in their own method groups.
type Workspace interface {
	ListWorkspaceFileChanges(ctx context.Context, in WorkspaceListQuery) (*Page[WorkspaceFileChange], error)
	GetWorkspaceDiff(ctx context.Context, in GetDiffRequest) (*Diff, error)
	GetWorkspaceFileHead(ctx context.Context, in GetFileHeadRequest) (*FileHead, error)
	GrepWorkspace(ctx context.Context, in GrepRequest) (*GrepResult, error)
	ListWorkspaceFiles(ctx context.Context, in ListFilesRequest) (*Page[FileEntry], error)
	ReadWorkspaceFile(ctx context.Context, in ReadFileRequest) (*FileContent, error)
	ListWorkspaceProjects(ctx context.Context, q PageQuery) (*Page[Project], error)
	// SubscribeWorkspace opens the non-run workspace event stream (AUX_API §3):
	// files/skills/mcp changes. Returns an ack + the event channel, closed when
	// the request ctx ends. Streaming method (in streamingMethods).
	SubscribeWorkspace(ctx context.Context, in WorkspaceSubscribeRequest) (*WorkspaceSubscribeResponse, <-chan WorkspaceEvent, error)
}
