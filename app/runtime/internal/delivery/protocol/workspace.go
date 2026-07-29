package protocol

import (
	"context"
	"iter"
)

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
}

// Runtime-wide change notification (§7). It is not a workspace concern: sessions,
// runs, goals and waiting sets change with no file involved, and a stream named after
// the workspace could only carry them by lying about what it was.
type RuntimeSubscription interface {
	// SubscribeRuntime opens the change-signal stream for the topics the caller asks
	// for. Returns an ack + the event sequence, ending when the request ctx ends.
	// Streaming method (in streamingMethods).
	SubscribeRuntime(ctx context.Context, in RuntimeSubscribeRequest) (*RuntimeSubscribeResponse, iter.Seq[RuntimeEvent], error)
}
