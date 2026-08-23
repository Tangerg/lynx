// Package agentmemory owns Lyra's agent-maintained long-term memory. It is a
// reviewed fact ledger, distinct from the human-authored LYRA.md cascade.
package agentmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	MaxContentBytes      = 8 << 10
	MaxVisiblePerTarget  = 512
	MaxRejectedPerTarget = 2_048
)

var (
	ErrNotFound        = errors.New("agentmemory: item not found")
	ErrNotPending      = errors.New("agentmemory: item is not pending")
	ErrDuplicate       = errors.New("agentmemory: duplicate content")
	ErrTargetFull      = errors.New("agentmemory: target is full")
	ErrInvalidMutation = errors.New("agentmemory: invalid mutation")
)

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

func (scope Scope) Valid() bool {
	return scope == ScopeProject || scope == ScopeUser
}

type Origin string

const (
	OriginAuto Origin = "auto"
	OriginUser Origin = "user"
)

func (origin Origin) Valid() bool {
	return origin == OriginAuto || origin == OriginUser
}

type Status string

const (
	StatusActive   Status = "active"
	StatusPending  Status = "pending"
	StatusRejected Status = "rejected"
)

func (status Status) Valid() bool {
	return status == StatusActive || status == StatusPending || status == StatusRejected
}

type ReviewDecision string

const (
	ReviewApprove ReviewDecision = "approve"
	ReviewReject  ReviewDecision = "reject"
)

type Item struct {
	ID        string
	Scope     Scope
	Project   string
	Content   string
	Digest    string
	Origin    Origin
	Status    Status
	Pinned    bool
	SessionID string
	Day       string
	Revision  uint64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewUserItem(
	id string,
	scope Scope,
	project string,
	content string,
	now time.Time,
) (Item, error) {
	content = strings.TrimSpace(content)
	now = now.UTC()
	item := Item{
		ID: id, Scope: scope, Project: project, Content: content,
		Digest: Digest(content), Origin: OriginUser, Status: StatusActive,
		Day:      now.Format(time.DateOnly),
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := item.Validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (item Item) Validate() error {
	switch {
	case strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ID) != item.ID:
		return errors.New("agentmemory: item id is required")
	case !item.Scope.Valid():
		return fmt.Errorf("agentmemory: invalid scope %q", item.Scope)
	case item.Scope == ScopeProject &&
		(!filepath.IsAbs(item.Project) || filepath.Clean(item.Project) != item.Project):
		return errors.New("agentmemory: project scope requires a canonical absolute project")
	case item.Scope == ScopeUser && item.Project != "":
		return errors.New("agentmemory: user scope forbids a project")
	case strings.TrimSpace(item.Content) == "" ||
		strings.TrimSpace(item.Content) != item.Content:
		return errors.New("agentmemory: content must be canonical non-empty text")
	case len(item.Content) > MaxContentBytes:
		return fmt.Errorf("agentmemory: content exceeds %d bytes", MaxContentBytes)
	case item.Digest != Digest(item.Content):
		return errors.New("agentmemory: content digest does not match")
	case !item.Origin.Valid():
		return fmt.Errorf("agentmemory: invalid origin %q", item.Origin)
	case !item.Status.Valid():
		return fmt.Errorf("agentmemory: invalid status %q", item.Status)
	case item.Origin == OriginUser && item.Status != StatusActive:
		return errors.New("agentmemory: user-authored item must be active")
	case item.Origin == OriginAuto &&
		(strings.TrimSpace(item.SessionID) == "" ||
			strings.TrimSpace(item.SessionID) != item.SessionID):
		return errors.New("agentmemory: automatic item requires a source session")
	case item.Origin == OriginUser && item.SessionID != "":
		return errors.New("agentmemory: user-authored item forbids a source session")
	case item.Pinned && item.Status != StatusActive:
		return errors.New("agentmemory: only active memory may be pinned")
	case item.Revision == 0:
		return errors.New("agentmemory: revision is required")
	case item.CreatedAt.IsZero() || item.UpdatedAt.IsZero():
		return errors.New("agentmemory: timestamps are required")
	case item.UpdatedAt.Before(item.CreatedAt):
		return errors.New("agentmemory: update precedes creation")
	}
	parsed, err := time.Parse(time.DateOnly, item.Day)
	if err != nil || parsed.Format(time.DateOnly) != item.Day {
		return errors.New("agentmemory: day must be an RFC 3339 date")
	}
	return nil
}

func (item Item) Review(decision ReviewDecision, now time.Time) (Item, error) {
	if item.Status != StatusPending {
		return Item{}, ErrNotPending
	}
	switch decision {
	case ReviewApprove:
		item.Status = StatusActive
	case ReviewReject:
		item.Status = StatusRejected
	default:
		return Item{}, fmt.Errorf("agentmemory: invalid review decision %q", decision)
	}
	item.Revision++
	item.UpdatedAt = now.UTC()
	return item, item.Validate()
}

type Patch struct {
	Content *string
	Pinned  *bool
}

func (item Item) Apply(patch Patch, now time.Time) (Item, bool, error) {
	if item.Status != StatusActive {
		return Item{}, false, fmt.Errorf(
			"%w: only active memory may be updated",
			ErrInvalidMutation,
		)
	}
	if patch.Content == nil && patch.Pinned == nil {
		return Item{}, false, fmt.Errorf("%w: update has no changes", ErrInvalidMutation)
	}
	changed := false
	if patch.Content != nil {
		content := strings.TrimSpace(*patch.Content)
		if content == "" {
			return Item{}, false, fmt.Errorf("%w: content is required", ErrInvalidMutation)
		}
		if content != item.Content {
			item.Content = content
			item.Digest = Digest(content)
			changed = true
		}
	}
	if patch.Pinned != nil && *patch.Pinned != item.Pinned {
		item.Pinned = *patch.Pinned
		changed = true
	}
	if !changed {
		return item, false, nil
	}
	item.Revision++
	item.UpdatedAt = now.UTC()
	if err := item.Validate(); err != nil {
		return Item{}, false, fmt.Errorf("%w: %v", ErrInvalidMutation, err)
	}
	return item, true, nil
}

func Digest(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}
