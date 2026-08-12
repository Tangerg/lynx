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

func (scope Scope) Validate() error {
	if scope != WorkingDirectory && scope != ProjectRoot && scope != Home {
		return fmt.Errorf("knowledge scope must be cwd, projectRoot, or home, got %q", scope)
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

func (target Target) Validate() error {
	if err := target.Scope.Validate(); err != nil {
		return err
	}
	if target.Scope == Home {
		if target.Workspace != "" {
			return errors.New("home knowledge does not belong to a workspace")
		}
		return nil
	}
	if target.Workspace == "" {
		return fmt.Errorf("%s knowledge requires a workspace", target.Scope)
	}
	return nil
}

type Entry struct {
	Scope     Scope
	Content   string
	Revision  string
	UpdatedAt *time.Time
}

func (entry Entry) Validate() error {
	if err := entry.Scope.Validate(); err != nil {
		return err
	}
	if entry.UpdatedAt != nil && entry.UpdatedAt.IsZero() {
		return errors.New("knowledge update time is zero")
	}
	if strings.TrimSpace(entry.Revision) == "" {
		return errors.New("knowledge revision is empty")
	}
	return nil
}

// Revise binds edited content to the exact document version the user opened.
// The runtime can therefore reject a stale editor instead of overwriting a
// concurrent change.
func (entry Entry) Revise(target Target, content string) (Update, error) {
	if err := entry.Validate(); err != nil {
		return Update{}, err
	}
	if err := target.Validate(); err != nil {
		return Update{}, err
	}
	if entry.Scope != target.Scope {
		return Update{}, fmt.Errorf("knowledge entry scope %s does not match target %s", entry.Scope, target.Scope)
	}
	return Update{Target: target, ExpectedRevision: entry.Revision, Content: content}, nil
}

type Update struct {
	Target           Target
	ExpectedRevision string
	Content          string
}

func (update Update) Validate() error {
	if err := update.Target.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(update.ExpectedRevision) == "" {
		return errors.New("knowledge update expected revision is empty")
	}
	return nil
}

type Service interface {
	Entries(context.Context, string) ([]Entry, error)
	Document(context.Context, Target) (Entry, error)
	Save(context.Context, Update) (Entry, error)
}
