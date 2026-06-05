// Package session defines the SessionService — Lyra's conversation
// lifecycle surface. Every multi-turn interaction lives under a Session;
// the service exposes the operations a client needs to find, resume,
// branch, or discard those conversations.
package session

import (
	"context"
	"time"
)

// IDPrefix is the type prefix every session id carries (API.md §2.2 —
// server-generated business ids are prefixed; mirrors the wire-side
// protocol.IDPrefixSession). Applied at generation so the id shape is
// identical regardless of backend.
const IDPrefix = "ses_"

// ForkAtMessageIDKey is the metadata key under which a forked session records
// the parent message it branched from. Stored on the child's Metadata so the
// branch point survives a round-trip through storage.
const ForkAtMessageIDKey = "fork_at_message_id"

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
	Title     string // human-readable; auto-generated from first user message
	Cwd       string // working-directory identity (API.md §0.2); defaults to the serve cwd
	Model     string // the model the session last explicitly ran against; empty ⇒ runtime default
	ParentID  string // empty for root sessions; non-empty for forks
	StartedAt time.Time
	UpdatedAt time.Time
	TurnCount int
	Metadata  map[string]string
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

// Fork derives a child session that branches from s at atMessageID. The child
// inherits s's working directory, takes s's title with a " (fork)" suffix, and
// records the branch point in metadata; ParentID points back at s. The parent's
// turn history, model and other accumulated state are NOT inherited — a fork
// starts a fresh conversation from the shared prefix.
//
// id and now are supplied by the caller: the storage adapter owns id generation
// (uuid) and the clock, keeping this derivation a pure, DB-free function the
// "what a fork is" rule can be unit-tested against.
func (s Session) Fork(id, atMessageID string, now time.Time) Session {
	return Session{
		ID:        id,
		Title:     s.Title + " (fork)",
		Cwd:       s.Cwd, // inherit the source's cwd (API.md §7.2)
		ParentID:  s.ID,
		StartedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]string{ForkAtMessageIDKey: atMessageID},
	}
}

// Service is the SessionService contract.
//
// All methods are safe for concurrent use. Implementations are
// transport-agnostic — HTTP/gRPC/IPC adapters wrap this surface
// without changing its shape.
type Service interface {
	// List returns every known session, newest-updated first.
	// Implementations may paginate; callers that need stability
	// supply Pagination opts.
	List(ctx context.Context) ([]Session, error)

	// Get returns the session by id, or ErrNotFound.
	Get(ctx context.Context, id string) (Session, error)

	// Create starts a fresh session; title is optional (auto-generated
	// from the first turn when empty). cwd is the session's working-
	// directory identity (API.md §0.2) — callers pass the serve cwd as
	// the default when the client omits it.
	Create(ctx context.Context, title, cwd string) (Session, error)

	// Fork creates a new session whose history equals the parent's
	// up to atMessageID, then diverges. The new session's ParentID
	// points at the parent.
	Fork(ctx context.Context, parentID, atMessageID string) (Session, error)

	// Delete drops the session and its persisted turns. Idempotent.
	Delete(ctx context.Context, id string) error

	// SetModel records the model a turn ran against, so sessions.list /
	// sessions.get surface the session's current model (the wire
	// Session.model). Called when a run explicitly selects a provider+model.
	// Returns ErrNotFound for an unknown id.
	SetModel(ctx context.Context, id, model string) error

	// Rename changes the session's human-readable title and refreshes
	// UpdatedAt. Backs sessions.update's title edit. The caller is
	// responsible for rejecting empty titles; this records whatever it is
	// given. Returns ErrNotFound for an unknown id.
	Rename(ctx context.Context, id, title string) error
}
