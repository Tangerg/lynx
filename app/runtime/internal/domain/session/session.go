// Package session owns Lyra's conversation identity and the behavior of one
// Session aggregate. Every multi-Run interaction lives under one Session.
// Persistence, clocks, identity generation, filesystem admission, and
// cross-aggregate orchestration remain consumer concerns.
package session

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// IDPrefix is the type prefix assigned when a Runtime generates a Session ID.
const IDPrefix = "ses_"

var (
	// ErrNotFound reports that no Session exists for an addressed identity.
	ErrNotFound = errors.New("session: not found")
	// ErrInvalid marks Session state or a requested edit that violates an
	// aggregate invariant.
	ErrInvalid = errors.New("session: invalid session")
	// ErrTitleRequired reports a Session edit with an empty title.
	ErrTitleRequired = errors.New("session: title required")
	// ErrRevisionConflict reports a stale optimistic-concurrency precondition.
	ErrRevisionConflict = errors.New("session: revision conflict")
)

// Draft contains the caller-decided identity and admitted values needed to
// start a root Session. ID generation, workspace resolution, and time acquisition
// occur before this value reaches the aggregate.
type Draft struct {
	ID        string
	Title     string
	CWD       string
	Model     string
	StartedAt time.Time
}

// Patch is the editable surface of a user-facing Session. Nil fields are left
// unchanged. ExpectedRevision is an optimistic-concurrency precondition rather
// than an editable field; zero deliberately omits that precondition for
// Runtime-owned maintenance.
type Patch struct {
	Title            *string
	Model            *string
	CWD              *string
	Favorite         *bool
	Isolated         *bool
	ExpectedRevision uint64
}

// Empty reports whether p requests no field change.
func (p Patch) Empty() bool {
	return p.Title == nil && p.Model == nil && p.CWD == nil && p.Favorite == nil && p.Isolated == nil
}

// Snapshot is the complete technical representation used only to reconstruct
// or persist a Session. It is not a mutation API.
type Snapshot struct {
	ID        string
	Title     string
	CWD       string
	Model     string
	ParentID  string
	StartedAt time.Time
	UpdatedAt time.Time
	Favorite  bool
	Isolated  bool
	Revision  uint64
}

// Session is one durable conversation aggregate. Its identity, lineage, and
// start time are immutable during ordinary edits; every mutable value changes
// through aggregate behavior and advances one monotonic revision.
type Session struct {
	id        string
	title     string
	cwd       string
	model     string
	parentID  string
	startedAt time.Time
	updatedAt time.Time
	favorite  bool
	isolated  bool
	revision  uint64
}

// New starts a root Session from caller-admitted values at revision one.
// Fresh interactive creation and scheduled creation share this same domain
// meaning; only the caller's source of their identity differs.
func New(draft Draft) (Session, error) {
	startedAt := canonicalTime(draft.StartedAt)
	snapshot := Snapshot{
		ID: draft.ID, Title: normalizeOptionalText(draft.Title),
		CWD: draft.CWD, Model: normalizeOptionalText(draft.Model),
		StartedAt: startedAt, UpdatedAt: startedAt, Revision: 1,
	}
	return Restore(snapshot)
}

// Restore reconstructs a Session from its complete technical representation
// while rechecking every aggregate invariant.
func Restore(snapshot Snapshot) (Session, error) {
	value := Session{
		id: snapshot.ID, title: snapshot.Title, cwd: snapshot.CWD,
		model: snapshot.Model, parentID: snapshot.ParentID,
		startedAt: canonicalTime(snapshot.StartedAt),
		updatedAt: canonicalTime(snapshot.UpdatedAt),
		favorite:  snapshot.Favorite, isolated: snapshot.Isolated,
		revision: snapshot.Revision,
	}
	if err := value.Validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

// Apply returns the Session produced by one edit. Text normalization and all
// preconditions are enforced on this single path, so callers cannot normalize a
// command and then bypass its aggregate checks. A semantic no-op returns s and
// changed=false without advancing revision or time.
func (s Session) Apply(patch Patch, updatedAt time.Time) (next Session, changed bool, err error) {
	if err := s.Validate(); err != nil {
		return Session{}, false, fmt.Errorf("%w: current state: %v", ErrInvalid, err)
	}
	if patch.ExpectedRevision != 0 && patch.ExpectedRevision != s.revision {
		return Session{}, false, ErrRevisionConflict
	}
	if patch.Empty() {
		return s, false, nil
	}

	next = s
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return Session{}, false, ErrTitleRequired
		}
		next.title = title
	}
	if patch.Model != nil {
		next.model = normalizeOptionalText(*patch.Model)
	}
	if patch.CWD != nil {
		if err := validateRequiredText("cwd", *patch.CWD); err != nil {
			return Session{}, false, err
		}
		next.cwd = *patch.CWD
	}
	if patch.Favorite != nil {
		next.favorite = *patch.Favorite
	}
	if patch.Isolated != nil {
		next.isolated = *patch.Isolated
	}
	if next.sameValue(s) {
		return s, false, nil
	}
	if err := next.advance(s, updatedAt); err != nil {
		return Session{}, false, err
	}
	return next, true, nil
}

// NameIfUntitled installs a generated initial title only while the Session is
// still untitled. A user title always wins. The returned bool reports whether a
// replacement was produced.
func (s Session) NameIfUntitled(title string, updatedAt time.Time) (Session, bool, error) {
	if strings.TrimSpace(s.title) != "" {
		return s, false, nil
	}
	return s.Apply(Patch{Title: &title, ExpectedRevision: s.revision}, updatedAt)
}

// Fork derives a new child Session. It inherits the admitted workspace and
// isolation choice, starts a fresh conversation with no model selection or
// favorite flag, and records immutable lineage back to s. An empty title uses
// the parent's human-readable fork title.
func (s Session) Fork(id, title string, startedAt time.Time) (Session, error) {
	if err := s.Validate(); err != nil {
		return Session{}, fmt.Errorf("%w: parent: %v", ErrInvalid, err)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		if s.title == "" {
			title = "Fork"
		} else {
			title = s.title + " (fork)"
		}
	}
	startedAt = canonicalTime(startedAt)
	return Restore(Snapshot{
		ID: id, Title: title, CWD: s.cwd, ParentID: s.id,
		StartedAt: startedAt, UpdatedAt: startedAt,
		Isolated: s.isolated, Revision: 1,
	})
}

// InstallRestoredWorkspace replaces an archive's workspace spelling with the
// canonical identity admitted before reconstruction. It is not an
// edit: revision and timestamps remain the archive's facts.
func (s Session) InstallRestoredWorkspace(cwd string) (Session, error) {
	if err := s.Validate(); err != nil {
		return Session{}, err
	}
	if err := validateRequiredText("cwd", cwd); err != nil {
		return Session{}, err
	}
	s.cwd = cwd
	return s, s.Validate()
}

// ReplaceWithRestore returns a replacement aggregate for an archive restored
// over an existing Session identity. The archive supplies the restored product
// values and immutable origin facts; the target aggregate owns the next
// revision and the caller supplies when the replacement occurred.
func (s Session) ReplaceWithRestore(restored Session, updatedAt time.Time) (Session, error) {
	if err := s.Validate(); err != nil {
		return Session{}, fmt.Errorf("%w: current state: %v", ErrInvalid, err)
	}
	if err := restored.Validate(); err != nil {
		return Session{}, fmt.Errorf("%w: restored state: %v", ErrInvalid, err)
	}
	if s.id != restored.id {
		return Session{}, fmt.Errorf("%w: restored identity %q differs from current identity %q", ErrInvalid, restored.id, s.id)
	}
	next := restored
	if err := next.advance(s, updatedAt); err != nil {
		return Session{}, err
	}
	return next, nil
}

func (s *Session) advance(previous Session, updatedAt time.Time) error {
	updatedAt = canonicalTime(updatedAt)
	if updatedAt.IsZero() {
		return fmt.Errorf("%w: update time is required", ErrInvalid)
	}
	if updatedAt.Before(previous.updatedAt) || updatedAt.Before(s.startedAt) {
		return fmt.Errorf("%w: update time precedes Session history", ErrInvalid)
	}
	if previous.revision == math.MaxUint64 {
		return fmt.Errorf("%w: revision overflow", ErrInvalid)
	}
	s.revision = previous.revision + 1
	s.updatedAt = updatedAt
	return s.Validate()
}

// Validate verifies identity, lineage, admitted values, time, and revision.
func (s Session) Validate() error {
	if err := validateRequiredText("id", s.id); err != nil {
		return err
	}
	if err := validateRequiredText("cwd", s.cwd); err != nil {
		return err
	}
	if s.title != strings.TrimSpace(s.title) || s.model != strings.TrimSpace(s.model) {
		return fmt.Errorf("%w: title and model must not contain surrounding whitespace", ErrInvalid)
	}
	if s.parentID != strings.TrimSpace(s.parentID) || s.parentID == s.id {
		return fmt.Errorf("%w: invalid parent identity", ErrInvalid)
	}
	if s.startedAt.IsZero() || s.updatedAt.IsZero() {
		return fmt.Errorf("%w: start and update times are required", ErrInvalid)
	}
	if s.updatedAt.Before(s.startedAt) {
		return fmt.Errorf("%w: update time precedes start time", ErrInvalid)
	}
	if s.revision == 0 {
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	}
	return nil
}

// Snapshot returns the complete technical representation of s.
func (s Session) Snapshot() Snapshot {
	return Snapshot{
		ID: s.id, Title: s.title, CWD: s.cwd, Model: s.model,
		ParentID: s.parentID, StartedAt: s.startedAt, UpdatedAt: s.updatedAt,
		Favorite: s.favorite, Isolated: s.isolated, Revision: s.revision,
	}
}

// ID returns the immutable Session identity.
func (s Session) ID() string { return s.id }

// Title returns the user-facing title, which may be empty before generation.
func (s Session) Title() string { return s.title }

// CWD returns the admitted canonical workspace identity.
func (s Session) CWD() string { return s.cwd }

// Model returns the last explicitly selected model, or empty for the Runtime default.
func (s Session) Model() string { return s.model }

// ParentID returns the immutable parent Session identity, or empty for a root.
func (s Session) ParentID() string { return s.parentID }

// StartedAt returns when the Session aggregate originated.
func (s Session) StartedAt() time.Time { return s.startedAt }

// UpdatedAt returns the time of the most recent aggregate replacement.
func (s Session) UpdatedAt() time.Time { return s.updatedAt }

// Favorite reports whether the user pinned the Session.
func (s Session) Favorite() bool { return s.favorite }

// Isolated reports whether Runs use a sandbox copy of the workspace.
func (s Session) Isolated() bool { return s.isolated }

// Revision returns the monotonic aggregate revision.
func (s Session) Revision() uint64 { return s.revision }

func (s Session) sameValue(other Session) bool {
	return s.id == other.id && s.title == other.title && s.cwd == other.cwd &&
		s.model == other.model && s.parentID == other.parentID &&
		s.startedAt.Equal(other.startedAt) && s.favorite == other.favorite &&
		s.isolated == other.isolated
}

func validateRequiredText(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: %s is required without surrounding whitespace", ErrInvalid, name)
	}
	return nil
}

func normalizeOptionalText(value string) string { return strings.TrimSpace(value) }

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}
