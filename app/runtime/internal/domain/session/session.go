// Package session models Lyra's conversation identity — the Session entity and
// the pure derivations over it (Fork and the editable Patch). Every multi-Run
// interaction lives under a Session. Persistence is a
// consumer concern: each coordinator defines the narrow store port it
// needs (list/resume/branch/discard), so this package holds no persistence
// interface of its own.
package session

import (
	"errors"
	"strings"
	"time"
)

// IDPrefix is the type prefix every session id carries. Applied at generation
// so the id shape is identical regardless of persistence backend.
const IDPrefix = "ses_"

// ErrTitleRequired reports a session edit with an empty title.
var ErrTitleRequired = errors.New("session: title required")

// ErrCwdUnavailable reports a session relocation target that is not an existing directory.
var ErrCwdUnavailable = errors.New("session: cwd unavailable")

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
// every Run against one Session id; restarting the
// runtime restores the Session from storage and lets execution continue
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
	ParentID  string // empty for root sessions; non-empty for a user-created fork
	StartedAt time.Time
	UpdatedAt time.Time
	Favorite  bool // user-pinned: sorts ahead of the rest in the session list
	// Isolated runs the session's tools inside a sandbox copy of Cwd instead of
	// the real working tree: fs + shell operate on the copy, the shell is
	// OS-jailed (network denied, $HOME hidden), and changes never touch the
	// project. Off by default. Requires a host isolation backend (macOS today).
	Isolated bool
	Revision uint64
}

// Fork derives a child session from s. The child inherits s's working
// directory, takes s's title with a " (fork)" suffix, and points ParentID back
// at s. The selected conversation prefix is copied separately;
// the parent's model and other accumulated state are not inherited.
//
// id and now are supplied by the caller, keeping this derivation pure and making
// the "what a fork is" rule directly testable.
func (s Session) Fork(id string, now time.Time) Session {
	return Session{
		ID:        id,
		Title:     s.Title + " (fork)",
		Cwd:       s.Cwd, // inherit the source's cwd (API.md §7.2)
		ParentID:  s.ID,
		Isolated:  s.Isolated, // a fork of an isolated session stays isolated
		StartedAt: now,
		UpdatedAt: now,
	}
}
