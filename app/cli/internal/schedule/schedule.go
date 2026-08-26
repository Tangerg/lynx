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

func (s Schedule) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("schedule id is empty")
	}
	if err := validateInstructionsAndCron(s.Instructions, s.Cron); err != nil {
		return fmt.Errorf("schedule %s: %w", s.ID, err)
	}
	if err := validateModelSelection(s.Provider, s.Model); err != nil {
		return fmt.Errorf("schedule %s: %w", s.ID, err)
	}
	if err := validateWorkspace(s.Workspace); err != nil {
		return fmt.Errorf("schedule %s: %w", s.ID, err)
	}
	if s.CreatedAt.IsZero() || s.Revision == 0 {
		return fmt.Errorf("schedule %s has incomplete persistence metadata", s.ID)
	}
	if s.Enabled != (s.NextRunAt != nil) {
		return fmt.Errorf("schedule %s has inconsistent enabled and next-run state", s.ID)
	}
	if s.LastRunAt != nil && s.LastRunAt.Before(s.CreatedAt) {
		return fmt.Errorf("schedule %s last ran before it was created", s.ID)
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

func (c Candidate) Validate() error {
	if err := validateInstructionsAndCron(c.Instructions, c.Cron); err != nil {
		return fmt.Errorf("schedule candidate: %w", err)
	}
	if err := validateModelSelection(c.Provider, c.Model); err != nil {
		return err
	}
	if err := validateWorkspace(c.Workspace); err != nil {
		return fmt.Errorf("schedule candidate: %w", err)
	}
	return nil
}

func (c Candidate) ValidateResult(result Schedule) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Title != c.Title {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, c.Title))
	}
	if result.Instructions != c.Instructions {
		problems = append(problems, fmt.Errorf("runtime returned instructions %q, want %q", result.Instructions, c.Instructions))
	}
	if c.Workspace != "" && result.Workspace != c.Workspace {
		problems = append(problems, fmt.Errorf("runtime returned workspace %q, want %q", result.Workspace, c.Workspace))
	}
	if result.Provider != c.Provider || result.Model != c.Model {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider, result.Model, c.Provider, c.Model,
		))
	}
	if result.Cron != c.Cron {
		problems = append(problems, fmt.Errorf("runtime returned cron %q, want %q", result.Cron, c.Cron))
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

func (w WorkspaceChange) Changed() bool { return w.mode != workspaceUnchanged }

func (w WorkspaceChange) Binding() (string, bool) {
	return w.path, w.mode == workspaceBound
}

func (w WorkspaceChange) UsesDefault() bool { return w.mode == workspaceDefault }

func (w WorkspaceChange) validate() error {
	switch w.mode {
	case workspaceUnchanged, workspaceDefault:
		if w.path != "" {
			return errors.New("schedule workspace change carries an unused path")
		}
	case workspaceBound:
		if w.path == "" {
			return errors.New("schedule workspace binding is empty")
		}
		if err := validateWorkspace(w.path); err != nil {
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

func (p Patch) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("schedule patch id is empty")
	}
	if p.ExpectedRevision == 0 {
		return errors.New("schedule patch expected revision is zero")
	}
	if !p.HasChanges() {
		return errors.New("schedule patch has no changes")
	}
	if p.Instructions != nil && strings.TrimSpace(*p.Instructions) == "" {
		return errors.New("schedule patch instructions are empty")
	}
	if p.Cron != nil && strings.TrimSpace(*p.Cron) == "" {
		return errors.New("schedule patch cron is empty")
	}
	if err := p.Workspace.validate(); err != nil {
		return err
	}
	if (p.Provider == nil) != (p.Model == nil) {
		return errors.New("schedule patch must replace provider and model together")
	}
	if p.Provider != nil {
		if err := validateModelSelection(*p.Provider, *p.Model); err != nil {
			return err
		}
	}
	return nil
}

func (p Patch) ValidateResult(result Schedule) error {
	if err := p.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID != p.ID {
		problems = append(problems, fmt.Errorf("runtime returned schedule %q, want %q", result.ID, p.ID))
	}
	if result.Revision <= p.ExpectedRevision {
		problems = append(problems, fmt.Errorf("runtime returned revision %d after expected revision %d", result.Revision, p.ExpectedRevision))
	}
	if p.Title != nil && result.Title != *p.Title {
		problems = append(problems, fmt.Errorf("runtime returned title %q, want %q", result.Title, *p.Title))
	}
	if p.Instructions != nil && result.Instructions != *p.Instructions {
		problems = append(problems, fmt.Errorf("runtime returned instructions %q, want %q", result.Instructions, *p.Instructions))
	}
	if path, bound := p.Workspace.Binding(); bound && result.Workspace != path {
		problems = append(problems, fmt.Errorf("runtime returned workspace %q, want %q", result.Workspace, path))
	} else if p.Workspace.UsesDefault() && result.Workspace != "" {
		problems = append(problems, fmt.Errorf("runtime kept workspace %q after restoring the default", result.Workspace))
	}
	if p.Provider != nil && (result.Provider != *p.Provider || result.Model != *p.Model) {
		problems = append(problems, fmt.Errorf(
			"runtime returned model %q/%q, want %q/%q",
			result.Provider, result.Model, *p.Provider, *p.Model,
		))
	}
	if p.Cron != nil && result.Cron != *p.Cron {
		problems = append(problems, fmt.Errorf("runtime returned cron %q, want %q", result.Cron, *p.Cron))
	}
	if p.Enabled != nil && result.Enabled != *p.Enabled {
		problems = append(problems, fmt.Errorf("runtime returned enabled %t, want %t", result.Enabled, *p.Enabled))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("schedule patch: %w", err)
	}
	return nil
}

func (p Patch) HasChanges() bool {
	return p.Title != nil || p.Instructions != nil || p.Workspace.Changed() ||
		p.Provider != nil || p.Model != nil || p.Cron != nil || p.Enabled != nil
}

// RunHandle identifies the headless session and run created by an immediate
// firing. The schedule's cron cursor is not advanced by this operation.
type RunHandle struct {
	SessionID string
	RunID     string
}

func (r RunHandle) Validate() error {
	if strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.RunID) == "" {
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
