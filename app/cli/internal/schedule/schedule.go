// Package schedule defines the CLI-owned scheduled-run entity and its runtime
// management port. Cron parsing and firing remain runtime responsibilities.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	if err := validateWorkspace(scheduled.Workspace); err != nil {
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
	if err := validateModelSelection(candidate.Provider, candidate.Model); err != nil {
		return err
	}
	if err := validateWorkspace(candidate.Workspace); err != nil {
		return fmt.Errorf("schedule candidate: %w", err)
	}
	return nil
}

func (candidate Candidate) ValidateResult(result Schedule) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Title != candidate.Title {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, candidate.Title))
	}
	if result.Instructions != candidate.Instructions {
		problems = append(problems, fmt.Errorf("runtime returned instructions %q, want %q", result.Instructions, candidate.Instructions))
	}
	if candidate.Workspace != "" && result.Workspace != candidate.Workspace {
		problems = append(problems, fmt.Errorf("runtime returned workspace %q, want %q", result.Workspace, candidate.Workspace))
	}
	if result.Provider != candidate.Provider || result.Model != candidate.Model {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider, result.Model, candidate.Provider, candidate.Model,
		))
	}
	if result.Cron != candidate.Cron {
		problems = append(problems, fmt.Errorf("runtime returned cron %q, want %q", result.Cron, candidate.Cron))
	}
	if !result.Enabled {
		problems = append(problems, errors.New("runtime returned a disabled new schedule"))
	}
	if result.LastRunAt != nil {
		problems = append(problems, errors.New("runtime returned prior run state for a new schedule"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("schedule candidate: %w", err)
	}
	return nil
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
		if err := validateWorkspace(change.path); err != nil {
			return fmt.Errorf("schedule workspace binding: %w", err)
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

func (patch Patch) ValidateResult(result Schedule) error {
	if err := patch.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID != patch.ID {
		problems = append(problems, fmt.Errorf("runtime returned schedule %q, want %q", result.ID, patch.ID))
	}
	if result.Revision <= patch.ExpectedRevision {
		problems = append(problems, fmt.Errorf("runtime returned revision %d after expected revision %d", result.Revision, patch.ExpectedRevision))
	}
	if patch.Title != nil && result.Title != *patch.Title {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, *patch.Title))
	}
	if patch.Instructions != nil && result.Instructions != *patch.Instructions {
		problems = append(problems, fmt.Errorf("runtime returned instructions %q, want %q", result.Instructions, *patch.Instructions))
	}
	if path, bound := patch.Workspace.Binding(); bound && result.Workspace != path {
		problems = append(problems, fmt.Errorf("runtime returned workspace %q, want %q", result.Workspace, path))
	} else if patch.Workspace.UsesDefault() && result.Workspace != "" {
		problems = append(problems, fmt.Errorf("runtime kept workspace %q after restoring the default", result.Workspace))
	}
	if patch.Provider != nil && (result.Provider != *patch.Provider || result.Model != *patch.Model) {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider, result.Model, *patch.Provider, *patch.Model,
		))
	}
	if patch.Cron != nil && result.Cron != *patch.Cron {
		problems = append(problems, fmt.Errorf("runtime returned cron %q, want %q", result.Cron, *patch.Cron))
	}
	if patch.Enabled != nil && result.Enabled != *patch.Enabled {
		problems = append(problems, fmt.Errorf("runtime returned enabled %t, want %t", result.Enabled, *patch.Enabled))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("schedule patch: %w", err)
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
	if provider != strings.TrimSpace(provider) || model != strings.TrimSpace(model) {
		return errors.New("schedule provider and model must not have surrounding whitespace")
	}
	return nil
}

func validateWorkspace(path string) error {
	if path != "" && !filepath.IsAbs(path) {
		return errors.New("workspace path is not absolute")
	}
	return nil
}
