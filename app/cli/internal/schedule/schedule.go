// Package schedule defines the CLI-owned scheduled-run entity and its runtime
// management port. Cron parsing and firing remain runtime responsibilities.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Schedule is one revisioned instruction set that the runtime fires on a cron
// trigger. Optional timestamps preserve the distinction between never fired,
// disabled, and scheduled.
type Schedule struct {
	ID           string
	Title        string
	Instructions string
	Workspace    string
	Provider     string
	Model        string
	Cron         string
	Enabled      bool
	LastRunAt    *time.Time
	NextRunAt    *time.Time
	CreatedAt    time.Time
	Revision     uint64
}

func (scheduled Schedule) Validate() error {
	if strings.TrimSpace(scheduled.ID) == "" {
		return errors.New("schedule id is empty")
	}
	if err := validateInstructionsAndCron(scheduled.Instructions, scheduled.Cron); err != nil {
		return fmt.Errorf("schedule %s: %w", scheduled.ID, err)
	}
	if err := validateModelSelection(scheduled.Provider, scheduled.Model); err != nil {
		return fmt.Errorf("schedule %s: %w", scheduled.ID, err)
	}
	if scheduled.CreatedAt.IsZero() || scheduled.Revision == 0 {
		return fmt.Errorf("schedule %s has incomplete persistence metadata", scheduled.ID)
	}
	if scheduled.Enabled != (scheduled.NextRunAt != nil) {
		return fmt.Errorf("schedule %s has inconsistent enabled and next-run state", scheduled.ID)
	}
	if scheduled.LastRunAt != nil && scheduled.LastRunAt.Before(scheduled.CreatedAt) {
		return fmt.Errorf("schedule %s last ran before it was created", scheduled.ID)
	}
	return nil
}

// Candidate is the complete configuration for a newly enabled schedule.
type Candidate struct {
	Title        string
	Instructions string
	Workspace    string
	Provider     string
	Model        string
	Cron         string
}

func (candidate Candidate) Validate() error {
	if err := validateInstructionsAndCron(candidate.Instructions, candidate.Cron); err != nil {
		return fmt.Errorf("schedule candidate: %w", err)
	}
	return validateModelSelection(candidate.Provider, candidate.Model)
}

// WorkspaceChange is a three-state schedule binding update. Its zero value
// preserves the current binding; constructors express either an explicit path
// or a return to the runtime's default workspace.
type WorkspaceChange struct {
	mode workspaceChangeMode
	path string
}

type workspaceChangeMode uint8

const (
	workspaceUnchanged workspaceChangeMode = iota
	workspaceBound
	workspaceDefault
)

func BindWorkspace(path string) WorkspaceChange {
	return WorkspaceChange{mode: workspaceBound, path: strings.TrimSpace(path)}
}

func UseDefaultWorkspace() WorkspaceChange {
	return WorkspaceChange{mode: workspaceDefault}
}

func (change WorkspaceChange) Changed() bool { return change.mode != workspaceUnchanged }

func (change WorkspaceChange) Binding() (string, bool) {
	return change.path, change.mode == workspaceBound
}

func (change WorkspaceChange) UsesDefault() bool { return change.mode == workspaceDefault }

func (change WorkspaceChange) validate() error {
	switch change.mode {
	case workspaceUnchanged, workspaceDefault:
		if change.path != "" {
			return errors.New("schedule workspace change carries an unused path")
		}
	case workspaceBound:
		if change.path == "" {
			return errors.New("schedule workspace binding is empty")
		}
	default:
		return errors.New("schedule workspace change mode is invalid")
	}
	return nil
}

// Patch is a revision-guarded partial update. Provider and Model are replaced
// together so the client cannot manufacture an incomplete model selection.
type Patch struct {
	ID               string
	ExpectedRevision uint64
	Title            *string
	Instructions     *string
	Workspace        WorkspaceChange
	Provider         *string
	Model            *string
	Cron             *string
	Enabled          *bool
}

func (patch Patch) Validate() error {
	if strings.TrimSpace(patch.ID) == "" {
		return errors.New("schedule patch id is empty")
	}
	if patch.ExpectedRevision == 0 {
		return errors.New("schedule patch expected revision is zero")
	}
	if !patch.HasChanges() {
		return errors.New("schedule patch has no changes")
	}
	if patch.Instructions != nil && strings.TrimSpace(*patch.Instructions) == "" {
		return errors.New("schedule patch instructions are empty")
	}
	if patch.Cron != nil && strings.TrimSpace(*patch.Cron) == "" {
		return errors.New("schedule patch cron is empty")
	}
	if err := patch.Workspace.validate(); err != nil {
		return err
	}
	if (patch.Provider == nil) != (patch.Model == nil) {
		return errors.New("schedule patch must replace provider and model together")
	}
	if patch.Provider != nil {
		if err := validateModelSelection(*patch.Provider, *patch.Model); err != nil {
			return err
		}
	}
	return nil
}

func (patch Patch) HasChanges() bool {
	return patch.Title != nil || patch.Instructions != nil || patch.Workspace.Changed() ||
		patch.Provider != nil || patch.Model != nil || patch.Cron != nil || patch.Enabled != nil
}

// RunHandle identifies the headless session and run created by an immediate
// firing. The schedule's cron cursor is not advanced by this operation.
type RunHandle struct {
	SessionID string
	RunID     string
}

func (handle RunHandle) Validate() error {
	if strings.TrimSpace(handle.SessionID) == "" || strings.TrimSpace(handle.RunID) == "" {
		return errors.New("schedule run handle is incomplete")
	}
	return nil
}

type Service interface {
	Schedules(context.Context) ([]Schedule, error)
	Create(context.Context, Candidate) (Schedule, error)
	Update(context.Context, Patch) (Schedule, error)
	Delete(context.Context, string) error
	RunNow(context.Context, string) (RunHandle, error)
}

func validateInstructionsAndCron(instructions, cron string) error {
	if strings.TrimSpace(instructions) == "" {
		return errors.New("instructions are empty")
	}
	if strings.TrimSpace(cron) == "" {
		return errors.New("cron is empty")
	}
	return nil
}

func validateModelSelection(provider, model string) error {
	if (strings.TrimSpace(provider) == "") != (strings.TrimSpace(model) == "") {
		return errors.New("schedule provider and model must both be set or both be empty")
	}
	return nil
}
