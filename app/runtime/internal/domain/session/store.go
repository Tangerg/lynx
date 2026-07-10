// Package session defines the Session store — Lyra's conversation
// lifecycle surface. Every multi-turn interaction lives under a Session;
// the store exposes the operations a client needs to find, resume,
// branch, or discard those conversations.
package session

import (
	"context"
	"errors"
	"strings"
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

// KindSubtask marks a session created for a sub-agent delegation (the `task`
// tool). Such a session has its OWN conversation history (isolated from the
// parent) and records the parent via [Session.ParentID], but it is internal:
// [Store.List] hides it so it never clutters the user's session list. The
// lineage stays queryable via [Store.Get] / [Store.Children].
const KindSubtask = "subtask"

// ErrTitleRequired reports a session edit with an empty title.
var ErrTitleRequired = errors.New("session: title required")

// ErrCwdUnavailable reports a session relocation target that is not an existing directory.
var ErrCwdUnavailable = errors.New("session: cwd unavailable")

// Patch is the editable surface of a user-facing session. Nil fields are
// ignored; non-nil fields replace the corresponding session value.
type Patch struct {
	Title    *string
	Model    *string
	Cwd      *string
	Metadata *map[string]any
	Favorite *bool
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
	Title     string // human-readable; auto-generated from first user message
	Cwd       string // working-directory identity (API.md §0.2); defaults to the serve cwd
	Model     string // the model the session last explicitly ran against; empty ⇒ runtime default
	ParentID  string // empty for root sessions; non-empty for a fork or a subtask child
	Kind      string // "" for a user-facing session (root / fork); KindSubtask for an internal delegation child
	StartedAt time.Time
	UpdatedAt time.Time
	Metadata  map[string]any // free-form, full-replaced by sessions.update (API.md §4.1, an object)
	Favorite  bool           // user-pinned: sorts ahead of the rest in the session list
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
		Metadata:  map[string]any{ForkAtMessageIDKey: atMessageID},
	}
}

// NewSubtask derives an internal delegation child of s — the sub-agent behind
// the `task` tool. The child inherits s's working directory, takes s's title
// with a " · subtask" suffix (just "subtask" when s is untitled), is marked
// [KindSubtask], and points ParentID back at s. Unlike [Fork] it records no
// branch point: a subtask is a fresh delegated conversation, not a branch of
// the parent's history.
//
// id and now are supplied by the caller — id is the agent runtime's child
// conversation id, so the persisted subtask history lines up. A parent that
// doesn't exist is passed as a zero Session carrying only its ID, which
// naturally yields the untitled-parent form; keeping the derivation here makes
// "what a subtask is" a pure, DB-free function the adapter just persists.
func (s Session) NewSubtask(id string, now time.Time) Session {
	title := "subtask"
	if s.Title != "" {
		title = s.Title + " · subtask"
	}
	return Session{
		ID:        id,
		Title:     title,
		Cwd:       s.Cwd,
		ParentID:  s.ID,
		Kind:      KindSubtask,
		StartedAt: now,
		UpdatedAt: now,
	}
}

// Store is the session persistence contract.
//
// All methods are safe for concurrent use. Implementations are
// transport-agnostic — HTTP/gRPC/IPC adapters wrap this surface
// without changing its shape.
type Store interface {
	// List returns every known session, newest-updated first. The full
	// list comes back in one call; the wire layer paginates on top.
	List(ctx context.Context) ([]Session, error)

	// Get returns the session by id, or ErrNotFound.
	Get(ctx context.Context, id string) (Session, error)

	// Create starts a fresh session; title is optional (auto-generated
	// from the first turn when empty). cwd is the session's working-
	// directory identity (API.md §0.2) — callers pass the serve cwd as
	// the default when the client omits it.
	Create(ctx context.Context, title, cwd string) (Session, error)

	// Restore upserts a session VERBATIM — its id, timestamps, and every
	// field as given — overwriting any existing row with the same id. It is
	// the write side of sessions.import (restore semantics): unlike Create it
	// preserves the supplied id rather than minting a new one. The caller owns
	// id validity.
	Restore(ctx context.Context, sess Session) error

	// Fork creates a new session whose history equals the parent's
	// up to atMessageID, then diverges. The new session's ParentID
	// points at the parent.
	Fork(ctx context.Context, parentID, atMessageID string) (Session, error)

	// CreateSubtask records an internal sub-agent delegation session under id
	// (the caller supplies it — it is the agent runtime's child conversation
	// id, so the persisted subtask history lines up), linked to parentID and
	// marked [KindSubtask]. The child inherits the parent's working directory.
	// Idempotent on id so a re-driven spawn doesn't error. Hidden from [List].
	CreateSubtask(ctx context.Context, id, parentID string) (Session, error)

	// Children returns the sessions whose ParentID is parentID — the
	// delegation/fork lineage under a session, newest-updated first. Includes
	// [KindSubtask] children (which List hides), so callers can walk the tree.
	Children(ctx context.Context, parentID string) ([]Session, error)

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

	// RenameIfUntitled sets the title only if the session currently has none
	// (an empty title), in one atomic UPDATE — the auto-titler's write, which
	// must not clobber a title the user set concurrently during the (slow,
	// async) title generation. A no-op (returns nil, not ErrNotFound) when the
	// session is already titled or unknown: titling is best-effort, so "nothing
	// to do" is success.
	RenameIfUntitled(ctx context.Context, id, title string) error

	// SetCwd relocates the session's working-directory identity (API.md §7.2
	// relocate): subsequent runs resolve tools and memory against the new cwd;
	// existing history is untouched. The caller validates the cwd exists
	// (cwd_unavailable is its concern); this records whatever it is given.
	// Returns ErrNotFound for an unknown id.
	SetCwd(ctx context.Context, id, cwd string) error

	// SetMetadata full-replaces the session's free-form metadata (API.md §7.2
	// — "全替换") and refreshes UpdatedAt. A nil map clears it. Returns
	// ErrNotFound for an unknown id.
	SetMetadata(ctx context.Context, id string, meta map[string]any) error

	// SetFavorite pins / unpins the session — a favorited session sorts ahead
	// of the rest in the session list. Backs sessions.update's favorite toggle.
	// Returns ErrNotFound for an unknown id.
	SetFavorite(ctx context.Context, id string, favorite bool) error
}
