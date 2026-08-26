// Package invalidation carries the "this moved, read it again" notices a use case
// publishes after its durable mutation commits.
//
// The facts are application-owned: which resource a committed write moved, and
// which of its members. Producers publish this semantic notice without choosing
// a transport topic or presentation shape.
//
// A notice carries IDs and nothing else. A value in it would be a second source of
// truth for something a query already answers, and the two would disagree the
// moment one notice was coalesced or dropped — which this channel is explicitly
// allowed to do.
package invalidation

// Resource is what moved. It is a closed set projected exhaustively at the
// publication boundary.
type Resource string

const (
	// Resync means a different Runtime process committed to the shared durable
	// store. The observer cannot recover the original use-case scope from
	// SQLite's commit counter, so every subscribed read model must be re-read.
	Resync Resource = "resync"
	// Sessions — a session was created, renamed, deleted, or its lifecycle moved.
	Sessions Resource = "sessions"
	// Runs — a run's lifecycle position changed (started, parked, resumed, ended).
	Runs Resource = "runs"
	// Interrupts — a waiting set opened, was answered, or was dropped.
	Interrupts Resource = "interrupts"
	// Goals — a session's autonomous goal changed.
	Goals Resource = "goals"
	// PlanState — the session-scoped Plan projection was committed.
	PlanState Resource = "plan"
	// Schedules — an editable scheduled run was created, updated, or deleted.
	Schedules Resource = "schedules"
	// Knowledge — a human-authored knowledge document was conditionally replaced.
	Knowledge Resource = "knowledge"
	// Hooks — a project's lifecycle-hook trust decision changed.
	Hooks Resource = "hooks"
	// Skills — the managed Skill library or proposal collection changed.
	Skills Resource = "skills"
	// MCP — an MCP server's durable configuration or live projection changed.
	MCP Resource = "mcp"
	// Models — provider configuration or a utility/embedding model role changed.
	Models Resource = "models"
	// Approvals — the default approval mode or remembered approval rules changed.
	Approvals Resource = "approvals"
	// AgentMemory — the agent-memory review collection changed.
	AgentMemory Resource = "agentMemory"
)

// Valid reports whether r belongs to the invalidation vocabulary.
func (r Resource) Valid() bool {
	return r == Resync || r == Sessions || r == Runs || r == Interrupts ||
		r == Goals || r == PlanState || r == Schedules || r == Knowledge ||
		r == Hooks || r == Skills || r == MCP || r == Models ||
		r == Approvals || r == AgentMemory
}

func (r Resource) String() string {
	if !r.Valid() {
		return "invalid"
	}
	return string(r)
}

// Notice is one committed change: the resource, and the members of it a reader can
// narrow to. Empty ID sets mean "every member of this resource may be stale",
// which is the honest answer when a mutation's scope is not enumerable.
type Notice struct {
	Resource    Resource
	SessionIDs  []string
	RunIDs      []string
	ScheduleIDs []string
	ServerIDs   []string
}

// InSession is the notice for a resource that moved inside one session, optionally
// naming the runs involved.
func InSession(resource Resource, sessionID string, runIDs ...string) Notice {
	return Notice{Resource: resource, SessionIDs: sessionIDs(sessionID), RunIDs: runIDs}
}

// InSessions is the notice for a resource that moved in several sessions at
// once, such as a bulk lifecycle mutation.
func InSessions(resource Resource, ids ...string) Notice {
	return Notice{Resource: resource, SessionIDs: ids}
}

// ForSchedules is the notice for committed changes to editable schedules.
func ForSchedules(ids ...string) Notice {
	return Notice{Resource: Schedules, ScheduleIDs: ids}
}

// ForMCP is the notice for MCP registry and live-connection changes.
func ForMCP(ids ...string) Notice {
	return Notice{Resource: MCP, ServerIDs: ids}
}

func sessionIDs(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}

// Publish is how a use case hands its notices out. It is a func rather than an
// interface because there is one method and nil is a meaningful value: a build with
// no runtime stream wired publishes nothing, and no producer has to know that.
type Publish func(Notice)

// Notify publishes each notice in order, and does nothing when no publisher is
// installed. Producers call this instead of guarding every site with a nil check.
func (p Publish) Notify(notices ...Notice) {
	if p == nil {
		return
	}
	for _, notice := range notices {
		p(notice)
	}
}
