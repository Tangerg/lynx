// Package session models Lyra's conversation identity — the Session entity and
// the pure derivations over it (Fork, NewSubtask, EffectiveModel, the editable
// Patch). Every multi-turn interaction lives under a Session. Persistence is a
// consumer concern: each coordinator/adapter defines the narrow store port it
// needs (list/resume/branch/discard), so this package holds no persistence
// interface of its own.
package session

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// IDPrefix is the type prefix every session id carries (API.md §2.2 —
// server-generated business ids are prefixed; mirrors the wire-side
// protocol.IDPrefixSession). Applied at generation so the id shape is
// identical regardless of backend.
const IDPrefix = "ses_"

// Kind identifies the lifecycle semantics of a persisted session.
type Kind string

const (
	// KindConversation is a user-facing root or forked conversation.
	KindConversation Kind = ""
	// KindSubtask marks a session created for a sub-agent delegation (the
	// `task` tool). Such a session has its own conversation history and is
	// hidden from the user-facing session list.
	KindSubtask Kind = "subtask"
)

// ParseKind rejects storage values the domain does not understand.
func ParseKind(value string) (Kind, error) {
	switch Kind(value) {
	case KindConversation, KindSubtask:
		return Kind(value), nil
	default:
		return KindConversation, fmt.Errorf("session: unknown kind %q", value)
	}
}

// ErrTitleRequired reports a session edit with an empty title.
var ErrTitleRequired = errors.New("session: title required")

// ErrCwdUnavailable reports a session relocation target that is not an existing directory.
var ErrCwdUnavailable = errors.New("session: cwd unavailable")

// ErrInvalidSubtask reports malformed delegated-conversation identity.
var ErrInvalidSubtask = errors.New("session: invalid subtask")

// ErrSubtaskConflict reports that a runtime child ID is already bound to a
// different product session identity.
var ErrSubtaskConflict = errors.New("session: subtask identity conflict")

// ErrRevisionConflict reports an optimistic-concurrency precondition that no
// longer matches the stored aggregate.
var ErrRevisionConflict = errors.New("session: revision conflict")

// Patch is the editable surface of a user-facing session. Nil fields are
// ignored; non-nil fields replace the corresponding session value.
type Patch struct {
	Title    *string
	Model    *string
	Cwd      *string
	Favorite *bool
	Isolated *bool
	// ExpectedRevision is the revision observed by the caller. Zero disables
	// the guard for runtime-owned maintenance writes such as SetModel.
	ExpectedRevision uint64
}

// Empty reports whether the patch carries no editable field. The revision is
// a precondition, not a change, and therefore does not make a patch non-empty.
func (p Patch) Empty() bool {
	return p.Title == nil && p.Model == nil && p.Cwd == nil && p.Favorite == nil && p.Isolated == nil
}

// Normalize returns a copy with domain-level text invariants applied.
func (p Patch) Normalize() (Patch, error) {
	if p.Title == nil {
		return p, nil
	}
	title := strings.TrimSpace(*p.Title)
	if title == "" {
		return Patch{}, ErrTitleRequired
	}
	p.Title = &title
	return p, nil
}

// Session is the persistent identity of a conversation. Lyra tracks
// every turn (chat exchange) against one Session id; restarting the
// runtime restores the Session from storage and lets a turn continue
// where it left off.
//
// Branching is a first-class operation — Sessions form a tree (any
// historical message can become the parent of a new branch), not a
// linear log. The tree shape is stored on disk so [Fork] can return
// without recomputing structure.
type Session struct {
	ID        string
	UserID    string // optional principal copied from the Agent runtime session
	AgentName string // exact compiled Agent identity for delegated conversations
	Title     string // human-readable; auto-generated from first user message
	Cwd       string // working-directory identity (API.md §0.2); defaults to the serve cwd
	Model     string // the model the session last explicitly ran against; empty ⇒ runtime default
	ParentID  string // empty for root sessions; non-empty for a fork or a subtask child
	Kind      Kind
	StartedAt time.Time
	UpdatedAt time.Time
	// DelegationMetadata belongs only to an internal delegated session. It is
	// opaque to the product model and never appears on the public Session API.
	DelegationMetadata DelegationMetadata
	Favorite           bool // user-pinned: sorts ahead of the rest in the session list
	// Isolated runs the session's tools inside a sandbox copy of Cwd instead of
	// the real working tree: fs + shell operate on the copy, the shell is
	// OS-jailed (network denied, $HOME hidden), and changes never touch the
	// project. Off by default. Requires a host isolation backend (macOS today).
	Isolated bool
	Revision uint64
}

// Subtask is the Agent runtime identity that must survive the product
// session's title/cwd enrichment and SQLite round trip.
type Subtask struct {
	ID                 string
	ParentID           string
	UserID             string
	AgentName          string
	StartedAt          time.Time
	UpdatedAt          time.Time
	DelegationMetadata DelegationMetadata
}

// Validate checks the identity and audit invariants required for a resumable
// delegated conversation.
func (s Subtask) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.ID) != s.ID {
		return fmt.Errorf("%w: ID must be non-empty without surrounding whitespace", ErrInvalidSubtask)
	}
	if strings.TrimSpace(s.ParentID) == "" || strings.TrimSpace(s.ParentID) != s.ParentID || s.ParentID == s.ID {
		return fmt.Errorf("%w: parent ID must be distinct and non-empty without surrounding whitespace", ErrInvalidSubtask)
	}
	if s.UserID != strings.TrimSpace(s.UserID) {
		return fmt.Errorf("%w: user ID has surrounding whitespace", ErrInvalidSubtask)
	}
	if strings.TrimSpace(s.AgentName) == "" || strings.TrimSpace(s.AgentName) != s.AgentName {
		return fmt.Errorf("%w: agent name must be non-empty without surrounding whitespace", ErrInvalidSubtask)
	}
	if s.StartedAt.IsZero() || s.UpdatedAt.IsZero() || s.UpdatedAt.Before(s.StartedAt) {
		return fmt.Errorf("%w: started and updated times must be ordered and non-zero", ErrInvalidSubtask)
	}
	return nil
}

// SameIdentity reports whether existing is the durable product projection of
// this delegated conversation. UpdatedAt and DelegationMetadata are mutable
// continuation data; the remaining runtime-owned fields are immutable identity.
func (s Subtask) SameIdentity(existing Session) bool {
	return existing.Kind == KindSubtask &&
		existing.ID == s.ID &&
		existing.ParentID == s.ParentID &&
		existing.UserID == s.UserID &&
		existing.AgentName == s.AgentName &&
		existing.StartedAt.Equal(s.StartedAt)
}

// EffectiveModel returns the model the session should report on the wire.
// A session that never explicitly ran against a provider+model (Model == "")
// falls back to the supplied runtime default, so callers — and the frontend,
// which derives the assistant's display name from it — always see a real model
// name. The fallback is a Session invariant, so it lives here rather than in
// the wire-translation layer.
func (s Session) EffectiveModel(defaultModel string) string {
	if s.Model != "" {
		return s.Model
	}
	return defaultModel
}

// Fork derives a child session from s. The child inherits s's working
// directory, takes s's title with a " (fork)" suffix, and points ParentID back
// at s. The application copies the selected conversation prefix separately;
// the parent's model and other accumulated state are not inherited.
//
// id and now are supplied by the caller: the storage adapter owns id generation
// (uuid) and the clock, keeping this derivation a pure, DB-free function the
// "what a fork is" rule can be unit-tested against.
func (s Session) Fork(id string, now time.Time) Session {
	return Session{
		ID:        id,
		UserID:    s.UserID,
		AgentName: s.AgentName,
		Title:     s.Title + " (fork)",
		Cwd:       s.Cwd, // inherit the source's cwd (API.md §7.2)
		ParentID:  s.ID,
		Isolated:  s.Isolated, // a fork of an isolated session stays isolated
		StartedAt: now,
		UpdatedAt: now,
	}
}

// NewSubtask derives an internal delegation child of s — the sub-agent behind
// the `task` tool. The child inherits s's working directory, takes s's title
// with a " · subtask" suffix (just "subtask" when s is untitled), is marked
// [KindSubtask], and points ParentID back at s. Unlike [Fork] it records no
// branch point: a subtask is a fresh delegated conversation, not a branch of
// the parent's history.
//
// The typed Subtask carries the runtime identity and timestamps; the receiver
// contributes only product-owned presentation state. A parent that does not
// exist is represented by an ID-only Session, which naturally yields the
// untitled-parent form. The derivation stays pure and DB-free.
func (s Session) NewSubtask(subtask Subtask) (Session, error) {
	if err := subtask.Validate(); err != nil {
		return Session{}, err
	}
	if s.ID != subtask.ParentID {
		return Session{}, fmt.Errorf("%w: parent %q does not match %q", ErrInvalidSubtask, s.ID, subtask.ParentID)
	}
	title := "subtask"
	if s.Title != "" {
		title = s.Title + " · subtask"
	}
	return Session{
		ID:                 subtask.ID,
		UserID:             subtask.UserID,
		AgentName:          subtask.AgentName,
		Title:              title,
		Cwd:                s.Cwd,
		ParentID:           s.ID,
		Kind:               KindSubtask,
		Isolated:           s.Isolated, // a subtask runs inside its parent's isolation
		StartedAt:          subtask.StartedAt,
		UpdatedAt:          subtask.UpdatedAt,
		DelegationMetadata: subtask.DelegationMetadata,
	}, nil
}
