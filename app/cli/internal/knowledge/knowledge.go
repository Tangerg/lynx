// Package knowledge defines the CLI's projection of the human-authored LYRA.md
// cascade. It is deliberately separate from runtime-maintained agent memory.
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Scope string

const (
	WorkingDirectory Scope = "cwd"
	ProjectRoot      Scope = "projectRoot"
	Home             Scope = "home"
)

func ParseScope(value string) (Scope, error) {
	scope := Scope(strings.TrimSpace(value))
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return scope, nil
}

func (s Scope) Validate() error {
	if s != WorkingDirectory && s != ProjectRoot && s != Home {
		return fmt.Errorf("knowledge scope must be cwd, projectRoot, or home, got %q", s)
	}
	return nil
}

// Target identifies exactly one document. Home is runtime-global and therefore
// intentionally carries no workspace; the other scopes cannot resolve without it.
type Target struct {
	Scope     Scope
	Workspace string
}

func NewTarget(scope Scope, workspace string) (Target, error) {
	target := Target{Scope: scope, Workspace: strings.TrimSpace(workspace)}
	return target, target.Validate()
}

func (t Target) Validate() error {
	if err := t.Scope.Validate(); err != nil {
		return err
	}
	if t.Scope == Home {
		if t.Workspace != "" {
			return errors.New("home knowledge does not belong to a workspace")
		}
		return nil
	}
	if t.Workspace == "" {
		return fmt.Errorf("%s knowledge requires a workspace", t.Scope)
	}
	return nil
}

type Entry struct {
	Scope     Scope
	Content   string
	Revision  string
	UpdatedAt *time.Time
}

func (e Entry) Validate() error {
	if err := e.Scope.Validate(); err != nil {
		return err
	}
	if e.UpdatedAt != nil && e.UpdatedAt.IsZero() {
		return errors.New("knowledge update time is zero")
	}
	if strings.TrimSpace(e.Revision) == "" {
		return errors.New("knowledge revision is empty")
	}
	return nil
}

// Revise binds edited content to the exact document version the user opened.
// The runtime can therefore reject a stale editor instead of overwriting a
// concurrent change.
func (e Entry) Revise(target Target, content string) (Update, error) {
	if err := e.Validate(); err != nil {
		return Update{}, err
	}
	if err := target.Validate(); err != nil {
		return Update{}, err
	}
	if e.Scope != target.Scope {
		return Update{}, fmt.Errorf("knowledge entry scope %s does not match target %s", e.Scope, target.Scope)
	}
	return Update{Target: target, ExpectedRevision: e.Revision, Content: content}, nil
}

type Update struct {
	Target           Target
	ExpectedRevision string
	Content          string
}

func (u Update) Validate() error {
	if err := u.Target.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(u.ExpectedRevision) == "" {
		return errors.New("knowledge update expected revision is empty")
	}
	return nil
}

type Service interface {
	Entries(context.Context, string) ([]Entry, error)
	Document(context.Context, Target) (Entry, error)
	Save(context.Context, Update) (Entry, error)
}
