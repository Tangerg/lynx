package execution

// WorkspaceMutation is the recorded intent of an in-flight file rollback. A Git
// reset updates multiple paths and is not atomic; a files+history rollback also
// spans two resources that cannot share an ACID transaction. The intent is
// therefore logged before the tree is touched and cleared only after every
// requested effect commits. A crash or incomplete reset is re-driven at boot.
//
// SessionID keys it: the per-session mutation slot admits at most one in-flight
// rollback per session, so a session has at most one pending intent. Cwd and
// ToRunID identify the file boundary; RestoreHistory says whether recovery also
// recomputes and applies the durable history cut.
type WorkspaceMutation struct {
	SessionID      string
	Cwd            string
	ToRunID        string
	RestoreHistory bool
}
