package session

// WorkspaceIdentity describes the current filesystem identity of a session's
// admitted working directory. Cwd remains stable even when the directory later
// disappears; ProjectRoot is the nearest repository root, or Cwd when no
// repository marker exists.
type WorkspaceIdentity struct {
	Cwd         string
	ProjectRoot string
	Missing     bool
}
