package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidSession reports a structurally invalid conversation identity.
var ErrInvalidSession = errors.New("session: invalid")

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

// BindAgent binds an unassigned session to agentName and rejects attempts to
// reuse a session with a different agent. The runtime calls it against the
// exact compiled deployment before dispatch.
func (s *Session) BindAgent(agentName string) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSession)
	}
	if strings.TrimSpace(agentName) == "" || strings.TrimSpace(agentName) != agentName {
		return fmt.Errorf("%w: agent name must be non-empty without surrounding whitespace", ErrInvalidSession)
	}
	if s.AgentName == "" {
		s.AgentName = agentName
		return nil
	}
	if s.AgentName != agentName {
		return fmt.Errorf("%w: session agent %q does not match deployment agent %q", ErrInvalidSession, s.AgentName, agentName)
	}
	return nil
}

// Validate checks identity, lineage, and audit-time invariants without
// mutating the session. Metadata encoding is checked by persistence backends
// at their JSON boundary.
func (s Session) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.ID) != s.ID {
		return fmt.Errorf("%w: ID must be non-empty without surrounding whitespace", ErrInvalidSession)
	}
	if s.ParentID != strings.TrimSpace(s.ParentID) || s.ParentID == s.ID {
		return fmt.Errorf("%w: invalid parent ID", ErrInvalidSession)
	}
	if s.UserID != strings.TrimSpace(s.UserID) {
		return fmt.Errorf("%w: user ID has surrounding whitespace", ErrInvalidSession)
	}
	if strings.TrimSpace(s.AgentName) == "" || strings.TrimSpace(s.AgentName) != s.AgentName {
		return fmt.Errorf("%w: agent name must be non-empty without surrounding whitespace", ErrInvalidSession)
	}
	if s.StartedAt.IsZero() || s.UpdatedAt.IsZero() || s.UpdatedAt.Before(s.StartedAt) {
		return fmt.Errorf("%w: started and updated times must be ordered and non-zero", ErrInvalidSession)
	}
	return nil
}

// Touch refreshes [Session.UpdatedAt] to now. [Engine.RunInSession]
// calls this on every successful dispatch so callers can rely on
// UpdatedAt as the last-activity timestamp.
func (s *Session) Touch() {
	s.UpdatedAt = time.Now()
}

// storageSnapshot returns an ownership-isolated representation of s matching
// the documented JSON persistence contract for Metadata. The JSON round trip
// both rejects values a durable SessionStore could not encode and recursively
// detaches nested maps and slices; a shallow map clone would still let callers
// mutate a saved session through one of its metadata values.
func (s Session) storageSnapshot() (Session, error) {
	if err := s.Validate(); err != nil {
		return Session{}, err
	}
	if s.Metadata == nil {
		return s, nil
	}
	encoded, err := json.Marshal(s.Metadata)
	if err != nil {
		return Session{}, fmt.Errorf("session metadata: %w", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return Session{}, fmt.Errorf("session metadata: %w", err)
	}
	s.Metadata = metadata
	return s, nil
}

// SessionReader loads durable conversation identity.
type SessionReader interface {
	// Load returns the session keyed by id, or a wrapped
	// [ErrSessionNotFound] when the id is unknown.
	Load(ctx context.Context, id string) (Session, error)
}

// SessionWriter persists a complete, valid conversation identity. Save must
// replace the record for the same ID and must not retain caller-owned mutable
// metadata.
type SessionWriter interface {
	Save(ctx context.Context, session Session) error
}

// SessionStore is the minimum persistence capability required by runtime.
// Implementations must be safe for concurrent use and must return ownership-
// isolated values. Administrative deletion and listing are optional
// capabilities rather than requirements imposed on every runtime store.
type SessionStore interface {
	SessionReader
	SessionWriter
}

// SessionDeleter is the optional idempotent cleanup capability.
type SessionDeleter interface {
	// Delete is idempotent — removing an unknown id is not an
	// error.
	Delete(ctx context.Context, id string) error
}

// SessionLister is the optional administrative listing capability.
type SessionLister interface {
	// List returns every session ID in a stable backend-defined order.
	List(ctx context.Context) ([]string, error)
}

// ErrSessionNotFound is the sentinel [SessionStore.Load] wraps when
// asked for an unknown id. Callers special-case via errors.Is.
var ErrSessionNotFound = errSessionNotFound{}

type errSessionNotFound struct{}

func (errSessionNotFound) Error() string { return "session store: session not found" }
