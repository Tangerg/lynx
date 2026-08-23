package protocol

import (
	"time"
)

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
