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

func (scope Scope) Validate() error {
	if scope != Project && scope != User {
		return fmt.Errorf("agent memory scope must be project or user, got %q", scope)
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

func (target Target) Validate() error {
	if err := target.Scope.Validate(); err != nil {
		return err
	}
	switch target.Scope {
	case Project:
		if target.Workspace == "" {
			return errors.New("project agent memory requires a workspace")
		}
		if !filepath.IsAbs(target.Workspace) {
			return errors.New("project agent memory workspace is not absolute")
		}
	case User:
		if target.Workspace != "" {
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

func (origin Origin) Validate() error {
	if origin != Automatic && origin != Authored {
		return fmt.Errorf("unknown agent memory origin %q", origin)
	}
	return nil
}

type Status string

const (
	Active  Status = "active"
	Pending Status = "pending"
)

func (status Status) Validate() error {
	if status != Active && status != Pending {
		return fmt.Errorf("unknown agent memory status %q", status)
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

func (item Item) Validate() error {
	if strings.TrimSpace(item.ID) == "" {
		return errors.New("agent memory item id is empty")
	}
	if err := item.Scope.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", item.ID, err)
	}
	if strings.TrimSpace(item.Content) == "" {
		return fmt.Errorf("agent memory item %s has empty content", item.ID)
	}
	if err := item.Origin.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", item.ID, err)
	}
	if err := item.Status.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", item.ID, err)
	}
	if item.Origin == Authored && item.Status != Active {
		return fmt.Errorf("agent memory item %s: user-authored memory must be active", item.ID)
	}
	if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		return fmt.Errorf("agent memory item %s has incomplete timestamps", item.ID)
	}
	if item.UpdatedAt.Before(item.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", item.ID)
	}
	return nil
}

type ReviewDecision string

const (
	Approve ReviewDecision = "approve"
	Reject  ReviewDecision = "reject"
)

func (decision ReviewDecision) Validate() error {
	if decision != Approve && decision != Reject {
		return fmt.Errorf("agent memory review decision must be approve or reject, got %q", decision)
	}
	return nil
}

// Patch changes one item without manufacturing a default for an omitted field.
type Patch struct {
	ID      string
	Content *string
	Pinned  *bool
}

func (patch Patch) Validate() error {
	if strings.TrimSpace(patch.ID) == "" {
		return errors.New("agent memory patch id is empty")
	}
	if patch.Content == nil && patch.Pinned == nil {
		return errors.New("agent memory patch has no changes")
	}
	if patch.Content != nil && strings.TrimSpace(*patch.Content) == "" {
		return errors.New("agent memory content is empty")
	}
	return nil
}

func (patch Patch) ValidateResult(result Item) error {
	if err := patch.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID != patch.ID {
		problems = append(problems, fmt.Errorf("runtime returned item %q, want %q", result.ID, patch.ID))
	}
	if patch.Content != nil && result.Content != strings.TrimSpace(*patch.Content) {
		problems = append(problems, fmt.Errorf("runtime returned content %q, want %q", result.Content, strings.TrimSpace(*patch.Content)))
	}
	if patch.Pinned != nil && result.Pinned != *patch.Pinned {
		problems = append(problems, fmt.Errorf("runtime returned pinned %t, want %t", result.Pinned, *patch.Pinned))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("agent memory patch: %w", err)
	}
	return nil
}

func (target Target) ValidateAddResult(content string, result Item) error {
	if err := target.Validate(); err != nil {
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
	if result.Scope != target.Scope {
		problems = append(problems, fmt.Errorf("runtime returned %s scope, want %s", result.Scope, target.Scope))
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
