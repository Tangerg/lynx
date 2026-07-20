package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrInvalidSession reports a structurally invalid conversation identity.
var ErrInvalidSession = errors.New("session: invalid")

// ErrInvalidSessionMetadata reports a value that cannot be represented by the
// session metadata JSON-object contract.
var ErrInvalidSessionMetadata = errors.New("session metadata: invalid")

// SessionMetadata is an owned JSON object associated with a session. Values
// are validated and encoded when set, so persistence cannot fail later because
// a caller stored a function, channel, cycle, or other non-JSON value. Its zero
// value is an empty object ready for use.
type SessionMetadata struct {
	fields map[string]json.RawMessage
}

// ParseSessionMetadata validates and owns one JSON object.
func ParseSessionMetadata(data []byte) (SessionMetadata, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return SessionMetadata{}, fmt.Errorf("%w: decode object: %w", ErrInvalidSessionMetadata, err)
	}
	if fields == nil {
		return SessionMetadata{}, fmt.Errorf("%w: expected object", ErrInvalidSessionMetadata)
	}
	metadata := SessionMetadata{fields: make(map[string]json.RawMessage, len(fields))}
	for name, value := range fields {
		metadata.fields[name] = bytes.Clone(value)
	}
	return metadata, nil
}

// Set validates and stores value under name.
func (m *SessionMetadata) Set(name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: encode field %q: %w", ErrInvalidSessionMetadata, name, err)
	}
	if m.fields == nil {
		m.fields = make(map[string]json.RawMessage)
	}
	m.fields[name] = encoded
	return nil
}

// Decode unmarshals the value associated with name into dst. It reports false
// without modifying dst when name is absent.
func (m SessionMetadata) Decode(name string, dst any) (bool, error) {
	value, ok := m.fields[name]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(value, dst); err != nil {
		return true, fmt.Errorf("decode session metadata field %q: %w", name, err)
	}
	return true, nil
}

// Delete removes name from m.
func (m *SessionMetadata) Delete(name string) { delete(m.fields, name) }

// Len returns the number of metadata fields.
func (m SessionMetadata) Len() int { return len(m.fields) }

// IsZero reports whether m is empty.
func (m SessionMetadata) IsZero() bool { return len(m.fields) == 0 }

// Clone returns a recursively ownership-isolated metadata value. Each field is
// already encoded JSON, so copying the raw messages also detaches nested data.
func (m SessionMetadata) Clone() SessionMetadata {
	if len(m.fields) == 0 {
		return SessionMetadata{}
	}
	clone := SessionMetadata{fields: make(map[string]json.RawMessage, len(m.fields))}
	for name, value := range m.fields {
		clone.fields[name] = bytes.Clone(value)
	}
	return clone
}

// MarshalJSON implements json.Marshaler. Empty metadata is encoded as an
// object rather than null.
func (m SessionMetadata) MarshalJSON() ([]byte, error) {
	if len(m.fields) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(m.fields)
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *SessionMetadata) UnmarshalJSON(data []byte) error {
	metadata, err := ParseSessionMetadata(data)
	if err != nil {
		return err
	}
	*m = metadata
	return nil
}

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

	// Metadata carries opaque, JSON-safe application annotations.
	Metadata SessionMetadata `json:"metadata,omitzero"`
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
// timestamps are set to now. Metadata's zero value is ready for use.
func NewSession(id, userID, agentName string) Session {
	now := time.Now()
	return Session{
		ID:        id,
		UserID:    userID,
		AgentName: agentName,
		StartedAt: now,
		UpdatedAt: now,
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

// storageSnapshot returns an ownership-isolated representation of s.
func (s Session) storageSnapshot() (Session, error) {
	if err := s.Validate(); err != nil {
		return Session{}, err
	}
	s.Metadata = s.Metadata.Clone()
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

// MemorySessionStore is the reference [SessionStore] backend — sessions live
// in a goroutine-safe map. It is suitable for tests and single-node
// development. Production deployments supply a persistent implementation.
type MemorySessionStore struct {
	store *memoryStore[Session]
}

// NewMemorySessionStore returns an empty session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		store: newMemoryStore[Session]("memory session store", ErrSessionNotFound),
	}
}

func (s *MemorySessionStore) Save(_ context.Context, session Session) error {
	snapshot, err := session.storageSnapshot()
	if err != nil {
		return fmt.Errorf("memory session store: snapshot %q: %w", session.ID, err)
	}
	return s.store.save(snapshot.ID, snapshot)
}

func (s *MemorySessionStore) Load(_ context.Context, id string) (Session, error) {
	stored, err := s.store.load(id)
	if err != nil {
		return Session{}, err
	}
	snapshot, err := stored.storageSnapshot()
	if err != nil {
		return Session{}, fmt.Errorf("memory session store: snapshot loaded session %q: %w", id, err)
	}
	return snapshot, nil
}

// Delete is idempotent.
func (s *MemorySessionStore) Delete(_ context.Context, id string) error {
	s.store.delete(id)
	return nil
}

// List returns every known session id in lexical order.
func (s *MemorySessionStore) List(_ context.Context) ([]string, error) {
	ids := s.store.list()
	slices.Sort(ids)
	return ids, nil
}
