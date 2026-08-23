// Package run owns the lifecycle of one logical agent Run across segments.
package run

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidTransition = errors.New("run: invalid transition")
	ErrNotFound          = errors.New("run: not found")
	ErrStaleSegment      = errors.New("run: stale segment")
)

type Status string

const (
	Running  Status = "running"
	Waiting  Status = "waiting"
	Finished Status = "finished"
)

type Outcome string

const (
	Completed Outcome = "completed"
	TimedOut  Outcome = "timedOut"
	Failed    Outcome = "failed"
	MaxSteps  Outcome = "maxSteps"
	MaxBudget Outcome = "maxBudget"
	Canceled  Outcome = "canceled"
	Lost      Outcome = "lost"
)

type Run struct {
	id              string
	sessionID       string
	parentRunID     string
	rootRunID       string
	spawnedByItemID string
	status          Status
	activeSegmentID string
	provider        string
	model           string
	outcome         Outcome
	detail          string
	createdAt       time.Time
	updatedAt       time.Time
	finishedAt      time.Time
}

type Record struct {
	Run  Run
	Body []byte
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Records []Record
	Next    *Cursor
}

type EventRecord struct {
	RunID, SegmentID, EventID string
	Ordinal                   int
	Body                      []byte
	CreatedAt                 time.Time
}

type Start struct {
	ID, SessionID, SegmentID string
	ParentRunID, RootRunID, SpawnedByItemID string
	Provider, Model string
	Now time.Time
}

func New(command Start) (Run, error) {
	value := Run{
		id: command.ID, sessionID: command.SessionID,
		parentRunID: command.ParentRunID, rootRunID: command.RootRunID,
		spawnedByItemID: command.SpawnedByItemID,
		status: Running, activeSegmentID: command.SegmentID,
		provider: command.Provider, model: command.Model,
		createdAt: command.Now.UTC(), updatedAt: command.Now.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Run{}, err
	}
	return value, nil
}

type Restore struct {
	ID, SessionID string
	ParentRunID, RootRunID, SpawnedByItemID string
	Status Status
	ActiveSegmentID, Provider, Model string
	Outcome Outcome
	Detail string
	CreatedAt, UpdatedAt, FinishedAt time.Time
}

func Rehydrate(state Restore) (Run, error) {
	value := Run{
		id: state.ID, sessionID: state.SessionID,
		parentRunID: state.ParentRunID, rootRunID: state.RootRunID,
		spawnedByItemID: state.SpawnedByItemID, status: state.Status,
		activeSegmentID: state.ActiveSegmentID, provider: state.Provider,
		model: state.Model, outcome: state.Outcome, detail: state.Detail,
		createdAt: state.CreatedAt.UTC(), updatedAt: state.UpdatedAt.UTC(),
		finishedAt: state.FinishedAt.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Run{}, err
	}
	return value, nil
}

func (value *Run) Wait(segmentID string, now time.Time) error {
	if err := value.requireActive(segmentID); err != nil {
		return err
	}
	value.status = Waiting
	value.activeSegmentID = ""
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Run) Resume(segmentID string, now time.Time) error {
	if value.status != Waiting || strings.TrimSpace(segmentID) == "" {
		return fmt.Errorf("%w: only a waiting run can resume", ErrInvalidTransition)
	}
	value.status = Running
	value.activeSegmentID = segmentID
	value.updatedAt = now.UTC()
	return value.Validate()
}

func (value *Run) Finish(segmentID string, outcome Outcome, detail string, now time.Time) error {
	if err := value.requireActive(segmentID); err != nil {
		return err
	}
	if !outcome.Valid() {
		return fmt.Errorf("%w: invalid outcome %q", ErrInvalidTransition, outcome)
	}
	value.status = Finished
	value.activeSegmentID = ""
	value.outcome = outcome
	value.detail = strings.TrimSpace(detail)
	value.updatedAt = now.UTC()
	value.finishedAt = now.UTC()
	return value.Validate()
}

func (value *Run) requireActive(segmentID string) error {
	if value.status != Running {
		return fmt.Errorf("%w: run is %s", ErrInvalidTransition, value.status)
	}
	if value.activeSegmentID != segmentID {
		return fmt.Errorf("%w: active %q, received %q", ErrStaleSegment, value.activeSegmentID, segmentID)
	}
	return nil
}

func (outcome Outcome) Valid() bool {
	switch outcome {
	case Completed, TimedOut, Failed, MaxSteps, MaxBudget, Canceled, Lost:
		return true
	default:
		return false
	}
}

func (value Run) Validate() error {
	lineage := value.parentRunID != "" || value.rootRunID != "" || value.spawnedByItemID != ""
	switch {
	case value.id == "" || value.sessionID == "":
		return fmt.Errorf("%w: identity is required", ErrInvalidTransition)
	case lineage && (value.parentRunID == "" || value.rootRunID == "" || value.spawnedByItemID == ""):
		return fmt.Errorf("%w: child lineage is incomplete", ErrInvalidTransition)
	case value.status == Running && value.activeSegmentID == "":
		return fmt.Errorf("%w: running run has no active segment", ErrInvalidTransition)
	case value.status != Running && value.activeSegmentID != "":
		return fmt.Errorf("%w: inactive run retains an active segment", ErrInvalidTransition)
	case value.status == Finished && (!value.outcome.Valid() || value.finishedAt.IsZero()):
		return fmt.Errorf("%w: finished run has no terminal fact", ErrInvalidTransition)
	case value.status != Finished && (value.outcome != "" || !value.finishedAt.IsZero()):
		return fmt.Errorf("%w: open run carries a terminal fact", ErrInvalidTransition)
	case value.createdAt.IsZero() || value.updatedAt.IsZero():
		return fmt.Errorf("%w: timestamps are required", ErrInvalidTransition)
	default:
		return nil
	}
}

func (value Run) ID() string              { return value.id }
func (value Run) SessionID() string       { return value.sessionID }
func (value Run) ParentRunID() string     { return value.parentRunID }
func (value Run) RootRunID() string       { return value.rootRunID }
func (value Run) SpawnedByItemID() string { return value.spawnedByItemID }
func (value Run) Status() Status           { return value.status }
func (value Run) ActiveSegmentID() string  { return value.activeSegmentID }
func (value Run) Provider() string         { return value.provider }
func (value Run) Model() string            { return value.model }
func (value Run) Outcome() Outcome         { return value.outcome }
func (value Run) Detail() string           { return value.detail }
func (value Run) CreatedAt() time.Time     { return value.createdAt }
func (value Run) UpdatedAt() time.Time     { return value.updatedAt }
func (value Run) FinishedAt() time.Time    { return value.finishedAt }
