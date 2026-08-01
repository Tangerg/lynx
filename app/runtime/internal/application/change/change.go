// Package change carries the "this moved, read it again" notices a use case
// publishes after its durable mutation commits.
//
// It lives in the application ring because the facts are the application's: which
// resource a committed write moved, and which of its members. Delivery owns the
// wire vocabulary those notices are projected onto (which topic, which state key),
// and the composition root connects the two — so a producer needs no knowledge of
// the protocol, and the protocol ring keeps naming no application type.
//
// A notice carries IDs and nothing else. A value in it would be a second source of
// truth for something a query already answers, and the two would disagree the
// moment one notice was coalesced or dropped — which this channel is explicitly
// allowed to do.
package change

// Resource is what moved. It is a closed set: [Resources] lists every value, and
// the projection onto the wire is checked against that list rather than trusting a
// switch to have kept up.
type Resource uint8

const (
	// Sessions — a session was created, renamed, deleted, or its lifecycle moved.
	Sessions Resource = iota + 1
	// Runs — a run's lifecycle position changed (started, parked, resumed, ended).
	Runs
	// Interrupts — a waiting set opened, was answered, or was dropped.
	Interrupts
	// Goals — a session's autonomous goal changed.
	Goals
	// TodoState — the session-scoped task list projection was committed.
	TodoState
)

// Resources is every Resource, in declaration order.
var Resources = []Resource{Sessions, Runs, Interrupts, Goals, TodoState}

// Notice is one committed change: the resource, and the members of it a reader can
// narrow to. Empty ID sets mean "every member of this resource may be stale",
// which is the honest answer when a mutation's scope is not enumerable.
type Notice struct {
	Resource   Resource
	SessionIDs []string
	RunIDs     []string
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
