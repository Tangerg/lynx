// Package session owns the durable user workspace conversation aggregate.
package session

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalid          = errors.New("session: invalid aggregate")
	ErrNotFound        = errors.New("session: not found")
	ErrRevisionConflict = errors.New("session: revision conflict")
)

type Cursor struct {
	UpdatedAt time.Time
	ID        string
	Favorite  bool
}

type Page struct {
	Projections []Projection
	Next        *Cursor
}

// Status is the current activity projected from the one open root Run tree. It
// is not persisted on Session: doing so would create a second lifecycle owner
// that could disagree with Run after a crash or concurrent transition.
type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusWaiting Status = "waiting"
)

func (status Status) Valid() bool {
	switch status {
	case StatusIdle, StatusRunning, StatusWaiting:
		return true
	default:
		return false
	}
}

// Projection is the Work Index read model: one durable Session aggregate plus
// its activity derived by the adapter in the same database read.
type Projection struct {
	Session Session
	Status  Status
}

func NewProjection(value Session, status Status) (Projection, error) {
	if err := value.Validate(); err != nil {
		return Projection{}, err
	}
	if !status.Valid() {
		return Projection{}, fmt.Errorf("%w: invalid projected status %q", ErrInvalid, status)
	}
	return Projection{Session: value, Status: status}, nil
}

type ID string

func (id ID) String() string { return string(id) }

type Workspace struct{ path string }

func NewWorkspace(path string) (Workspace, error) {
	if !filepath.IsAbs(path) {
		return Workspace{}, fmt.Errorf("%w: workspace must be absolute", ErrInvalid)
	}
	clean := filepath.Clean(path)
	if clean == string(filepath.Separator) {
		return Workspace{}, fmt.Errorf("%w: filesystem root is not a workspace", ErrInvalid)
	}
	return Workspace{path: clean}, nil
}

func (workspace Workspace) Path() string { return workspace.path }

type Session struct {
	id        ID
	title     string
	workspace Workspace
	model     string
	favorite  bool
	revision  uint64
	createdAt time.Time
	updatedAt time.Time
}

type Create struct {
	ID        ID
	Title     string
	Workspace Workspace
	Model     string
	Now       time.Time
}

func New(command Create) (Session, error) {
	title := strings.TrimSpace(command.Title)
	if title == "" {
		title = "New session"
	}
	value := Session{
		id: command.ID, title: title, workspace: command.Workspace,
		model: strings.TrimSpace(command.Model), revision: 1,
		createdAt: command.Now.UTC(), updatedAt: command.Now.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

type Restore struct {
	ID ID
	Title string
	Workspace Workspace
	Model string
	Favorite bool
	Revision uint64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func Rehydrate(state Restore) (Session, error) {
	value := Session{
		id: state.ID, title: state.Title, workspace: state.Workspace,
		model: state.Model, favorite: state.Favorite, revision: state.Revision,
		createdAt: state.CreatedAt.UTC(), updatedAt: state.UpdatedAt.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

type Patch struct {
	ExpectedRevision uint64
	Title             *string
	Workspace         *Workspace
	Model             *string
	Favorite          *bool
	Now               time.Time
}

func (value *Session) Update(patch Patch) error {
	if value.revision != patch.ExpectedRevision {
		return fmt.Errorf("%w: have %d, expected %d", ErrRevisionConflict, value.revision, patch.ExpectedRevision)
	}
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return fmt.Errorf("%w: title is empty", ErrInvalid)
		}
		value.title = title
	}
	if patch.Workspace != nil {
		value.workspace = *patch.Workspace
	}
	if patch.Model != nil {
		value.model = strings.TrimSpace(*patch.Model)
	}
	if patch.Favorite != nil {
		value.favorite = *patch.Favorite
	}
	value.revision++
	value.updatedAt = patch.Now.UTC()
	return value.Validate()
}

func (value Session) Validate() error {
	switch {
	case value.id == "":
		return fmt.Errorf("%w: id is required", ErrInvalid)
	case strings.TrimSpace(value.title) == "":
		return fmt.Errorf("%w: title is required", ErrInvalid)
	case value.workspace.path == "":
		return fmt.Errorf("%w: workspace is required", ErrInvalid)
	case value.revision == 0:
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	case value.createdAt.IsZero() || value.updatedAt.IsZero():
		return fmt.Errorf("%w: timestamps are required", ErrInvalid)
	case value.updatedAt.Before(value.createdAt):
		return fmt.Errorf("%w: updatedAt precedes createdAt", ErrInvalid)
	default:
		return nil
	}
}

func (value Session) ID() ID                 { return value.id }
func (value Session) Title() string          { return value.title }
func (value Session) Workspace() Workspace   { return value.workspace }
func (value Session) Model() string          { return value.model }
func (value Session) Favorite() bool         { return value.favorite }
func (value Session) Revision() uint64        { return value.revision }
func (value Session) CreatedAt() time.Time    { return value.createdAt }
func (value Session) UpdatedAt() time.Time    { return value.updatedAt }
