package protocol

import (
	"context"
	"iter"
	"time"
)

// Workspace is the workspace.* and workspaces.* method surface. Workspace-scoped
// methods carry an explicit WorkspaceRef; resolve/list are the discovery roots.
type Workspace interface {
	ResolveWorkspace(ctx context.Context, in ResolveWorkspaceRequest) (*WorkspaceInfo, error)
	ListWorkspaces(ctx context.Context) (*Page[WorkspaceSummary], error)
	ListWorkspaceFileChanges(ctx context.Context, in WorkspaceQuery) (*Page[WorkspaceFileChange], error)
	GetWorkspaceDiff(ctx context.Context, in GetDiffRequest) (*Diff, error)
	GetWorkspaceFileHead(ctx context.Context, in GetFileHeadRequest) (*FileHead, error)
	GrepWorkspace(ctx context.Context, in GrepRequest) (*GrepResult, error)
	ListWorkspaceFiles(ctx context.Context, in ListFilesRequest) (*Page[FileEntry], error)
	ReadWorkspaceFile(ctx context.Context, in ReadFileRequest) (*FileContent, error)
}

// RuntimeSubscription is the runtime-wide change notification surface. It is
// separate from workspace scope because sessions, runs, goals, and interrupts may
// change without a filesystem workspace changing.
type RuntimeSubscription interface {
	SubscribeRuntime(ctx context.Context, in RuntimeSubscribeRequest) (*RuntimeSubscribeResponse, iter.Seq[RuntimeEvent], error)
}

// WorkspaceRef is the stable wire reference to a filesystem workspace. Path is
// absolute and server-canonical in responses. Requests pass a reference rather
// than a loose cwd string so workspace scope has one reusable shape everywhere.
type WorkspaceRef struct {
	Path string `json:"path"`
}

// WorkspaceAvailability is the live filesystem state of an admitted reference.
type WorkspaceAvailability string

const (
	WorkspaceAvailable WorkspaceAvailability = "available"
	WorkspaceMissing   WorkspaceAvailability = "missing"
)

// WorkspaceInfo is the server-resolved identity of one filesystem workspace.
// ProjectRoot is the nearest repository root, or Ref.Path outside a repository.
type WorkspaceInfo struct {
	Ref          WorkspaceRef          `json:"ref"`
	ProjectRoot  string                `json:"projectRoot,omitempty"`
	Availability WorkspaceAvailability `json:"availability"`
}

// WorkspaceSummary adds the user-facing session catalog facts to a Workspace.
type WorkspaceSummary struct {
	Workspace    WorkspaceInfo `json:"workspace"`
	Name         string        `json:"name"`
	SessionCount int           `json:"sessionCount"`
	LastActiveAt *time.Time    `json:"lastActiveAt,omitzero"`
}

// ResolveWorkspaceRequest resolves Ref, or the runtime's default workspace when
// Ref is omitted. Omission is allowed only here and on session creation; scoped
// business requests always carry a concrete WorkspaceRef.
type ResolveWorkspaceRequest struct {
	Ref *WorkspaceRef `json:"ref,omitempty"`
}
