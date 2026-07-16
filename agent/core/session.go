package core

import (
	"context"
	"time"
)

// Session models a multi-turn conversation against an agent. The
// session id doubles as the chat history conversation id so the
// per-turn message history is automatically loaded + persisted by
// history middleware — no extra
// wiring needed beyond installing the middleware on the chat client.
//
// Sessions carry identity + audit metadata; message history stays behind
// the chat history Store abstraction, keeping the session struct thin.
//
// Sessions are persisted via [SessionStore]; the in-memory reference
// implementation ([NewMemorySessionStore]) ships in this package.
// Production deployments wire a persistent backend (postgres / redis
// / mongo / ...) under the same interface.
type Session struct {
	// ID uniquely identifies the conversation. Doubles as the
	// chat history conversation id so message
	// history flows through without separate plumbing.
	ID string `json:"id"`

	// ParentID links a child session to the one that spawned it. A
	// sub-agent (e.g. a subtask delegation) runs under its OWN session —
	// its conversation history is isolated from the parent's — but records
	// the parent's session id here so the delegation lineage is preserved.
	// Empty for a root session. The runtime stamps it when spawning a child
	// process under a parent that has a conversation (see the spawn path).
	ParentID string `json:"parent_id,omitempty"`

	// UserID identifies the principal driving the conversation.
	// Optional — present for multi-tenant deployments / audit
	// trails / RBAC; absent for anonymous / single-user use.
	UserID string `json:"user_id,omitempty"`

	// AgentName binds the session to a specific agent definition.
	// [Engine.RunInSession] uses this to dispatch new turns when
	// the caller doesn't supply an agent explicitly.
	AgentName string `json:"agent_name"`

	// StartedAt is the wall-clock time of session creation.
	StartedAt time.Time `json:"started_at"`

	// UpdatedAt is the wall-clock time of the most recent activity
	// ([Engine.RunInSession] refreshes this on every turn).
	UpdatedAt time.Time `json:"updated_at"`

	// Metadata carries free-form annotations the application
	// wants to associate with the session (channel name, locale,
	// preference flags, etc.). The runtime treats this as
	// opaque — backends marshal it via [encoding/json], so only
	// JSON-friendly values round-trip.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SessionInfo is the immutable identity/audit subset actions may inspect.
// Mutable host metadata and the Session pointer itself stay outside
// ProcessContext so an action cannot rewrite routing or persistence state.
type SessionInfo struct {
	ID        string
	ParentID  string
	UserID    string
	AgentName string
	StartedAt time.Time
	UpdatedAt time.Time
}

func (s *Session) info() (SessionInfo, bool) {
	if s == nil {
		return SessionInfo{}, false
	}
	return SessionInfo{
		ID:        s.ID,
		ParentID:  s.ParentID,
		UserID:    s.UserID,
		AgentName: s.AgentName,
		StartedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
	}, true
}

// NewSession builds a session with sensible defaults — the caller's id
// is stored verbatim (callers are expected to seed a stable id) and
// timestamps are set to now. The Metadata map is allocated so callers
// can write without nil-checking.
func NewSession(id, userID, agentName string) Session {
	now := time.Now()
	return Session{
		ID:        id,
		UserID:    userID,
		AgentName: agentName,
		StartedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]any{},
	}
}

// Touch refreshes [Session.UpdatedAt] to now. [Engine.RunInSession]
// calls this on every successful dispatch so callers can rely on
// UpdatedAt as the last-activity timestamp.
func (s *Session) Touch() {
	s.UpdatedAt = time.Now()
}

// SessionStore is the persistence SPI for [Session] records. The
// shape mirrors [ProcessStore]: Save / Load / Delete / List. The
// in-memory reference implementation lives in
// [MemorySessionStore]; persistent backends are the caller's to
// supply behind the same interface.
//
// All methods are expected to be safe for concurrent use.
type SessionStore interface {
	Save(ctx context.Context, session Session) error

	// Load returns the session keyed by id, or a wrapped
	// [ErrSessionNotFound] when the id is unknown.
	Load(ctx context.Context, id string) (Session, error)

	// Delete is idempotent — removing an unknown id is not an
	// error.
	Delete(ctx context.Context, id string) error

	// List returns every session id. Backends that paginate
	// naturally may return a stable subset and let callers
	// iterate — the interface does not dictate pagination.
	List(ctx context.Context) ([]string, error)
}

// ErrSessionNotFound is the sentinel [SessionStore.Load] wraps when
// asked for an unknown id. Callers special-case via errors.Is.
var ErrSessionNotFound = errSessionNotFound{}

type errSessionNotFound struct{}

func (errSessionNotFound) Error() string { return "session store: session not found" }
