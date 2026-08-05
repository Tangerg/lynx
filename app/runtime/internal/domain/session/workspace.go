package session

// WorkspaceIdentity describes the current filesystem identity of a session's
// admitted working directory. CWD remains stable even when the directory later
// disappears; ProjectRoot is the nearest repository root, or CWD when no
// repository marker exists.
type WorkspaceIdentity struct {
	CWD         string
	ProjectRoot string
	Missing     bool
}
