// Package agentmemory defines the CLI's projection of runtime-maintained facts
// and the use cases through which a user governs them.
package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Scope is the durable partition that owns a memory item.
type Scope string

const (
	Project Scope = "project"
	User    Scope = "user"
)

func ParseScope(value string) (Scope, error) {
	scope := Scope(strings.TrimSpace(value))
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return scope, nil
}

func (s Scope) Validate() error {
	if s != Project && s != User {
		return fmt.Errorf("agent memory scope must be project or user, got %q", s)
	}
	return nil
}

// Target couples a scope to exactly the workspace context it requires.
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
	switch t.Scope {
	case Project:
		if t.Workspace == "" {
			return errors.New("project agent memory requires a workspace")
		}
		if !filepath.IsAbs(t.Workspace) {
			return errors.New("project agent memory workspace is not absolute")
		}
	case User:
		if t.Workspace != "" {
			return errors.New("user agent memory does not belong to a workspace")
		}
	}
	return nil
}

type Origin string

const (
	Automatic Origin = "auto"
	Authored  Origin = "user"
)

func (o Origin) Validate() error {
	if o != Automatic && o != Authored {
		return fmt.Errorf("unknown agent memory origin %q", o)
	}
	return nil
}

type Status string

const (
	Active  Status = "active"
	Pending Status = "pending"
)

func (s Status) Validate() error {
	if s != Active && s != Pending {
		return fmt.Errorf("unknown agent memory status %q", s)
	}
	return nil
}

// Item is one stable, addressable fact together with its review provenance.
type Item struct {
	ID        string
	Scope     Scope
	Content   string
	Origin    Origin
	Status    Status
	Pinned    bool
	SessionID string
	Day       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (i Item) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return errors.New("agent memory item id is empty")
	}
	if err := i.Scope.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", i.ID, err)
	}
	if strings.TrimSpace(i.Content) == "" {
		return fmt.Errorf("agent memory item %s has empty content", i.ID)
	}
	if err := i.Origin.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", i.ID, err)
	}
	if err := i.Status.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", i.ID, err)
	}
	if i.Origin == Authored && i.Status != Active {
		return fmt.Errorf("agent memory item %s: user-authored memory must be active", i.ID)
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() {
		return fmt.Errorf("agent memory item %s has incomplete timestamps", i.ID)
	}
	if i.UpdatedAt.Before(i.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", i.ID)
	}
	return nil
}

type ReviewDecision string

const (
	Approve ReviewDecision = "approve"
	Reject  ReviewDecision = "reject"
)

func (r ReviewDecision) Validate() error {
	if r != Approve && r != Reject {
		return fmt.Errorf("agent memory review decision must be approve or reject, got %q", r)
	}
	return nil
}

// Patch changes one item without manufacturing a default for an omitted field.
type Patch struct {
	ID      string
	Content *string
	Pinned  *bool
}

func (p Patch) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("agent memory patch id is empty")
	}
	if p.Content == nil && p.Pinned == nil {
		return errors.New("agent memory patch has no changes")
	}
	if p.Content != nil && strings.TrimSpace(*p.Content) == "" {
		return errors.New("agent memory content is empty")
	}
	return nil
}

func (p Patch) ValidateResult(result Item) error {
	if err := p.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID != p.ID {
		problems = append(problems, fmt.Errorf("runtime returned item %q, want %q", result.ID, p.ID))
	}
	if p.Content != nil && result.Content != strings.TrimSpace(*p.Content) {
		problems = append(problems, fmt.Errorf("runtime returned content %q, want %q", result.Content, strings.TrimSpace(*p.Content)))
	}
	if p.Pinned != nil && result.Pinned != *p.Pinned {
		problems = append(problems, fmt.Errorf("runtime returned pinned %t, want %t", result.Pinned, *p.Pinned))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("agent memory patch: %w", err)
	}
	return nil
}

func (t Target) ValidateAddResult(content string, result Item) error {
	if err := t.Validate(); err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("add agent memory: content is empty")
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Scope != t.Scope {
		problems = append(problems, fmt.Errorf("runtime returned %s scope, want %s", result.Scope, t.Scope))
	}
	if result.Content != content {
		problems = append(problems, fmt.Errorf("runtime returned content %q, want %q", result.Content, content))
	}
	if result.Origin != Authored || result.Status != Active {
		problems = append(problems, fmt.Errorf(
			"runtime returned %s/%s provenance, want %s/%s",
			result.Origin, result.Status, Authored, Active,
		))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("add agent memory: %w", err)
	}
	return nil
}

type Service interface {
	Items(context.Context, Target) ([]Item, error)
	Review(context.Context, string, ReviewDecision) error
	Update(context.Context, Patch) (Item, error)
	Delete(context.Context, string) error
	Add(context.Context, Target, string) (Item, error)
}
