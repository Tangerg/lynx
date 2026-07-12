package execution

// WorkspaceMutation is the recorded intent of an in-flight file rollback — the
// one runtime operation that mutates two resources that can't share an ACID
// transaction: the working tree (git) and the durable history (SQLite). §8.5:
// the intent is logged before either is touched and cleared once both commit, so
// a crash mid-operation is re-driven at boot (reentrant restore + idempotent
// truncation) rather than leaving the tree and the history disagreeing.
//
// SessionID keys it: the per-session mutation slot admits at most one in-flight
// rollback per session, so a session has at most one pending intent. Cwd and
// ToRunID are all recovery needs to re-drive the operation — restore the tree to
// the run snapshot and recompute the durable cut from the run boundary.
type WorkspaceMutation struct {
	SessionID string
	Cwd       string
	ToRunID   string
}
