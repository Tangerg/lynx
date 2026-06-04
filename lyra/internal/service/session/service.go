// Package session defines the SessionService — Lyra's conversation
// lifecycle surface. Every multi-turn interaction lives under a Session;
// the service exposes the operations a client needs to find, resume,
// branch, or discard those conversations.
package session

import (
	"context"
	"time"
)

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
	ParentID  string // empty for root sessions; non-empty for forks
	StartedAt time.Time
	UpdatedAt time.Time
	TurnCount int
	Metadata  map[string]string
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
}
