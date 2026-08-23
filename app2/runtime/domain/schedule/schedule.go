// Package schedule owns recurring Run intent, cadence, revision, and durable
// occurrence identity. Timer loops, Runtime events, and Run admission remain
// application concerns.
package schedule

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
)

const (
	maxIdentityBytes     = 512
	maxTitleBytes        = 512
	maxInstructionsBytes = 64 << 10
	maxCronBytes         = 512
)

var (
	ErrInvalid          = errors.New("schedule: invalid aggregate")
	ErrNotFound         = errors.New("schedule: not found")
	ErrRevisionConflict = errors.New("schedule: revision conflict")
	ErrNotDue           = errors.New("schedule: not due")
)

type State struct {
	ID           string
	Title        string
	Instructions string
	Workspace    string
	Provider     string
	Model        string
	Cron         string
	Enabled      bool
	LastRunAt    time.Time
	NextRunAt    time.Time
	Revision     uint64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Schedule struct {
	state     State
	selection modelselection.Selection
}

type Create struct {
	ID           string
	Title        string
	Instructions string
	Workspace    string
	Selection    modelselection.Selection
	Cron         string
	Now          time.Time
}

func New(command Create) (Schedule, error) {
	now := command.Now.UTC()
	next, err := NextRun(command.Cron, now)
	if err != nil {
		return Schedule{}, err
	}
	state := State{
		ID: command.ID, Title: normalizeTitle(command.Title),
		Instructions: command.Instructions, Workspace: command.Workspace,
		Provider: command.Selection.Provider(), Model: command.Selection.Model(),
		Cron: strings.TrimSpace(command.Cron), Enabled: true, NextRunAt: next,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	return Rehydrate(state)
}

func Rehydrate(state State) (Schedule, error) {
	state.LastRunAt = state.LastRunAt.UTC()
	state.NextRunAt = state.NextRunAt.UTC()
	state.CreatedAt = state.CreatedAt.UTC()
	state.UpdatedAt = state.UpdatedAt.UTC()
	selection, err := modelselection.New(state.Provider, state.Model)
	if err != nil {
		return Schedule{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	value := Schedule{state: state, selection: selection}
	if err := value.validate(); err != nil {
		return Schedule{}, err
	}
	return value, nil
}

func (value Schedule) validate() error {
	state := value.state
	if !validIdentity(state.ID) || !strings.HasPrefix(state.ID, "sch_") {
		return fmt.Errorf("%w: unsafe identity", ErrInvalid)
	}
	if state.Title != normalizeTitle(state.Title) || len(state.Title) > maxTitleBytes ||
		!utf8.ValidString(state.Title) || strings.IndexFunc(state.Title, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: unsafe title", ErrInvalid)
	}
	if strings.TrimSpace(state.Instructions) == "" || len(state.Instructions) > maxInstructionsBytes ||
		!utf8.ValidString(state.Instructions) || strings.ContainsRune(state.Instructions, 0) {
		return fmt.Errorf("%w: unsafe instructions", ErrInvalid)
	}
	if state.Workspace != "" && (!filepath.IsAbs(state.Workspace) ||
		filepath.Clean(state.Workspace) != state.Workspace || state.Workspace == string(filepath.Separator)) {
		return fmt.Errorf("%w: workspace is not canonical", ErrInvalid)
	}
	if state.Cron != strings.TrimSpace(state.Cron) || len(state.Cron) > maxCronBytes {
		return fmt.Errorf("%w: unsafe cron expression", ErrInvalid)
	}
	if _, err := parseCron(state.Cron); err != nil {
		return err
	}
	if state.Enabled == state.NextRunAt.IsZero() {
		return fmt.Errorf("%w: enabled state and next run disagree", ErrInvalid)
	}
	if state.Revision == 0 || state.CreatedAt.IsZero() || state.UpdatedAt.Before(state.CreatedAt) ||
		(!state.LastRunAt.IsZero() && state.LastRunAt.Before(state.CreatedAt)) {
		return fmt.Errorf("%w: invalid revision or timestamps", ErrInvalid)
	}
	return nil
}

type Patch struct {
	Title        *string
	Instructions *string
	Workspace    *string
	Selection    *modelselection.Selection
	Cron         *string
	Enabled      *bool
}

func (value Schedule) Update(expected uint64, patch Patch, now time.Time) (Schedule, bool, error) {
	if expected != value.state.Revision {
		return Schedule{}, false, ErrRevisionConflict
	}
	next := value.state
	selection := value.selection
	cronChanged := false
	wasEnabled := next.Enabled
	if patch.Title != nil {
		next.Title = normalizeTitle(*patch.Title)
	}
	if patch.Instructions != nil {
		next.Instructions = *patch.Instructions
	}
	if patch.Workspace != nil {
		next.Workspace = *patch.Workspace
	}
	if patch.Selection != nil {
		selection = *patch.Selection
		next.Provider, next.Model = selection.Provider(), selection.Model()
	}
	if patch.Cron != nil {
		next.Cron = strings.TrimSpace(*patch.Cron)
		cronChanged = next.Cron != value.state.Cron
	}
	if patch.Enabled != nil {
		next.Enabled = *patch.Enabled
	}
	if sameEditableState(value.state, next) {
		return value, false, nil
	}
	transitionAt := now.UTC()
	if transitionAt.Before(value.state.UpdatedAt) {
		transitionAt = value.state.UpdatedAt
	}
	if !next.Enabled {
		next.NextRunAt = time.Time{}
	} else if !wasEnabled || cronChanged {
		candidate, err := NextRun(next.Cron, transitionAt)
		if err != nil {
			return Schedule{}, false, err
		}
		next.NextRunAt = candidate
	}
	next.Revision++
	next.UpdatedAt = transitionAt
	updated, err := Rehydrate(next)
	if err != nil {
		return Schedule{}, false, err
	}
	updated.selection = selection
	return updated, true, nil
}

func (value Schedule) RecordRun(ranAt, now time.Time) (Schedule, bool, error) {
	ranAt = ranAt.UTC()
	if !value.state.LastRunAt.IsZero() && !ranAt.After(value.state.LastRunAt) {
		return value, false, nil
	}
	transitionAt := now.UTC()
	if transitionAt.Before(value.state.UpdatedAt) {
		transitionAt = value.state.UpdatedAt
	}
	next := value.state
	next.LastRunAt = ranAt
	next.Revision++
	next.UpdatedAt = transitionAt
	updated, err := Rehydrate(next)
	return updated, true, err
}

func (value Schedule) Due(at time.Time) bool {
	return value.state.Enabled && !value.state.NextRunAt.IsZero() && !value.state.NextRunAt.After(at.UTC())
}

func NextRun(expression string, after time.Time) (time.Time, error) {
	parsed, err := parseCron(strings.TrimSpace(expression))
	if err != nil {
		return time.Time{}, err
	}
	return parsed.Next(after.UTC()).UTC(), nil
}

func parseCron(expression string) (cron.Schedule, error) {
	if expression == "" {
		return nil, fmt.Errorf("%w: cron expression is required", ErrInvalid)
	}
	parsed, err := cron.ParseStandard(expression)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cron expression: %v", ErrInvalid, err)
	}
	return parsed, nil
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Schedules []Schedule
	Next      *Cursor
}

type OccurrenceStatus string

const (
	OccurrencePending  OccurrenceStatus = "pending"
	OccurrenceAccepted OccurrenceStatus = "accepted"
)

type OccurrenceState struct {
	ID         string
	Schedule   State
	DueAt      time.Time
	FiredAt    time.Time
	NextRunAt  time.Time
	SessionID  string
	RunID      string
	Status     OccurrenceStatus
	AcceptedAt time.Time
}

type Occurrence struct {
	state    OccurrenceState
	schedule Schedule
}

func NewOccurrence(value Schedule, firedAt time.Time, sessionID, runID string) (Occurrence, error) {
	firedAt = firedAt.UTC()
	if !value.Due(firedAt) {
		return Occurrence{}, ErrNotDue
	}
	next, err := NextRun(value.state.Cron, firedAt)
	if err != nil {
		return Occurrence{}, err
	}
	state := OccurrenceState{
		Schedule: value.State(), DueAt: value.state.NextRunAt, FiredAt: firedAt,
		NextRunAt: next, SessionID: sessionID, RunID: runID, Status: OccurrencePending,
	}
	state.ID = occurrenceID(value.state.ID, state.DueAt)
	return RehydrateOccurrence(state)
}

func RehydrateOccurrence(state OccurrenceState) (Occurrence, error) {
	state.DueAt = state.DueAt.UTC()
	state.FiredAt = state.FiredAt.UTC()
	state.NextRunAt = state.NextRunAt.UTC()
	state.AcceptedAt = state.AcceptedAt.UTC()
	value, err := Rehydrate(state.Schedule)
	if err != nil {
		return Occurrence{}, err
	}
	if state.ID != occurrenceID(value.ID(), state.DueAt) || !validIdentity(state.SessionID) ||
		!strings.HasPrefix(state.SessionID, "ses_") || !validIdentity(state.RunID) ||
		!strings.HasPrefix(state.RunID, "run_") || state.DueAt.IsZero() || state.FiredAt.Before(state.DueAt) ||
		!state.NextRunAt.After(state.FiredAt) {
		return Occurrence{}, fmt.Errorf("%w: invalid occurrence identity or timestamps", ErrInvalid)
	}
	switch state.Status {
	case OccurrencePending:
		if !state.AcceptedAt.IsZero() {
			return Occurrence{}, fmt.Errorf("%w: pending occurrence is already accepted", ErrInvalid)
		}
	case OccurrenceAccepted:
		if state.AcceptedAt.IsZero() || state.AcceptedAt.Before(state.FiredAt) {
			return Occurrence{}, fmt.Errorf("%w: accepted occurrence has no acceptance time", ErrInvalid)
		}
	default:
		return Occurrence{}, fmt.Errorf("%w: unknown occurrence status", ErrInvalid)
	}
	return Occurrence{state: state, schedule: value}, nil
}

func (value Occurrence) ClaimedSchedule() (Schedule, error) {
	next := value.schedule.state
	next.NextRunAt = value.state.NextRunAt
	next.Revision++
	next.UpdatedAt = value.state.FiredAt
	if next.UpdatedAt.Before(value.schedule.state.UpdatedAt) {
		next.UpdatedAt = value.schedule.state.UpdatedAt
	}
	return Rehydrate(next)
}

func (value Schedule) State() State                         { return value.state }
func (value Schedule) ID() string                           { return value.state.ID }
func (value Schedule) Title() string                        { return value.state.Title }
func (value Schedule) Instructions() string                 { return value.state.Instructions }
func (value Schedule) Workspace() string                    { return value.state.Workspace }
func (value Schedule) Selection() modelselection.Selection  { return value.selection }
func (value Schedule) Cron() string                         { return value.state.Cron }
func (value Schedule) Enabled() bool                        { return value.state.Enabled }
func (value Schedule) LastRunAt() time.Time                 { return value.state.LastRunAt }
func (value Schedule) NextRunAt() time.Time                 { return value.state.NextRunAt }
func (value Schedule) Revision() uint64                     { return value.state.Revision }
func (value Schedule) CreatedAt() time.Time                 { return value.state.CreatedAt }
func (value Schedule) UpdatedAt() time.Time                 { return value.state.UpdatedAt }
func (value Occurrence) State() OccurrenceState             { return value.state }
func (value Occurrence) ID() string                         { return value.state.ID }
func (value Occurrence) Schedule() Schedule                 { return value.schedule }
func (value Occurrence) DueAt() time.Time                   { return value.state.DueAt }
func (value Occurrence) FiredAt() time.Time                 { return value.state.FiredAt }
func (value Occurrence) NextRunAt() time.Time               { return value.state.NextRunAt }
func (value Occurrence) SessionID() string                  { return value.state.SessionID }
func (value Occurrence) RunID() string                      { return value.state.RunID }

func normalizeTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Scheduled task"
	}
	return value
}

func sameEditableState(left, right State) bool {
	return left.Title == right.Title && left.Instructions == right.Instructions &&
		left.Workspace == right.Workspace && left.Provider == right.Provider && left.Model == right.Model &&
		left.Cron == right.Cron && left.Enabled == right.Enabled
}

func occurrenceID(scheduleID string, dueAt time.Time) string {
	digest := sha256.Sum256([]byte(scheduleID + "\x00" + dueAt.UTC().Format(time.RFC3339Nano)))
	return "fir_" + hex.EncodeToString(digest[:12])
}

func validIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maxIdentityBytes &&
		utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
