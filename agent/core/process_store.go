package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ProcessSnapshotSchemaVersion is the only durable process wire schema this
// development version accepts. Missing and unknown versions fail explicitly;
// the framework never guesses an obsolete snapshot shape.
const ProcessSnapshotSchemaVersion uint16 = 4

var (
	ErrSnapshotNotFound = errors.New("process store: snapshot not found")
	ErrSnapshotSchema   = errors.New("process snapshot: unsupported schema")
	ErrInvalidSnapshot  = errors.New("process snapshot: invalid")
)

// ProcessSnapshot is the complete durable state required to inspect or resume
// one process. OwnCost, OwnTokens, OwnModelCalls, and OwnEmbeddingCalls contain
// only this process's direct ledger; descendants persist their own snapshots,
// and runtime reconstructs aggregate usage through parent-child links.
// Runtime-only objects, derived world state, functions, and closures are
// intentionally absent.
type ProcessSnapshot struct {
	SchemaVersion uint16 `json:"schema_version"`

	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Depth    int    `json:"depth,omitempty"`

	Deployment DeploymentRef `json:"deployment"`
	StartedAt  time.Time     `json:"started_at"`
	CapturedAt time.Time     `json:"captured_at"`
	Status     ProcessStatus `json:"-"`

	Suspension *interaction.Suspension `json:"suspension,omitempty"`
	GoalName   string                  `json:"goal_name,omitempty"`
	History    []ActionRunSnapshot     `json:"history,omitempty"`
	Failure    string                  `json:"failure,omitempty"`

	OwnCost   float64 `json:"own_cost"`
	OwnTokens int     `json:"own_tokens"`

	OwnModelCalls     []ModelCall     `json:"own_model_calls,omitempty"`
	OwnEmbeddingCalls []EmbeddingCall `json:"own_embedding_calls,omitempty"`

	Blackboard map[string]TaggedValue `json:"blackboard,omitempty"`
	Conditions map[string]bool        `json:"conditions,omitempty"`
	Objects    []TaggedValue          `json:"objects,omitempty"`
}

type processSnapshotWire struct {
	SchemaVersion     uint16                  `json:"schema_version"`
	ID                string                  `json:"id"`
	ParentID          string                  `json:"parent_id,omitempty"`
	Depth             int                     `json:"depth,omitempty"`
	Deployment        DeploymentRef           `json:"deployment"`
	StartedAt         time.Time               `json:"started_at"`
	CapturedAt        time.Time               `json:"captured_at"`
	Status            string                  `json:"status"`
	Suspension        *interaction.Suspension `json:"suspension,omitempty"`
	GoalName          string                  `json:"goal_name,omitempty"`
	History           []ActionRunSnapshot     `json:"history,omitempty"`
	Failure           string                  `json:"failure,omitempty"`
	OwnCost           float64                 `json:"own_cost"`
	OwnTokens         int                     `json:"own_tokens"`
	OwnModelCalls     []ModelCall             `json:"own_model_calls,omitempty"`
	OwnEmbeddingCalls []EmbeddingCall         `json:"own_embedding_calls,omitempty"`
	Blackboard        map[string]TaggedValue  `json:"blackboard,omitempty"`
	Conditions        map[string]bool         `json:"conditions,omitempty"`
	Objects           []TaggedValue           `json:"objects,omitempty"`
}

func (s ProcessSnapshot) wire() processSnapshotWire {
	return processSnapshotWire{
		SchemaVersion: s.SchemaVersion,
		ID:            s.ID, ParentID: s.ParentID, Depth: s.Depth,
		Deployment: s.Deployment, StartedAt: s.StartedAt, CapturedAt: s.CapturedAt,
		Status: s.Status.String(), Suspension: s.Suspension, GoalName: s.GoalName,
		History: s.History, Failure: s.Failure, OwnCost: s.OwnCost, OwnTokens: s.OwnTokens,
		OwnModelCalls: s.OwnModelCalls, OwnEmbeddingCalls: s.OwnEmbeddingCalls,
		Blackboard: s.Blackboard, Conditions: s.Conditions, Objects: s.Objects,
	}
}

func (w processSnapshotWire) snapshot() (ProcessSnapshot, error) {
	status, err := parseProcessStatus(w.Status)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	return ProcessSnapshot{
		SchemaVersion: w.SchemaVersion,
		ID:            w.ID, ParentID: w.ParentID, Depth: w.Depth,
		Deployment: w.Deployment, StartedAt: w.StartedAt, CapturedAt: w.CapturedAt,
		Status: status, Suspension: w.Suspension, GoalName: w.GoalName,
		History: w.History, Failure: w.Failure, OwnCost: w.OwnCost, OwnTokens: w.OwnTokens,
		OwnModelCalls: w.OwnModelCalls, OwnEmbeddingCalls: w.OwnEmbeddingCalls,
		Blackboard: w.Blackboard, Conditions: w.Conditions, Objects: w.Objects,
	}, nil
}

// Validate checks the durable process state without mutating it.
func (s ProcessSnapshot) Validate() error {
	if s.SchemaVersion != ProcessSnapshotSchemaVersion {
		return fmt.Errorf("%w: version %d", ErrSnapshotSchema, s.SchemaVersion)
	}
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.ID) != s.ID {
		return fmt.Errorf("%w: ID must be non-empty without surrounding whitespace", ErrInvalidSnapshot)
	}
	if s.ParentID != strings.TrimSpace(s.ParentID) || s.ParentID == s.ID {
		return fmt.Errorf("%w: invalid parent_id", ErrInvalidSnapshot)
	}
	if s.Depth < 0 {
		return fmt.Errorf("%w: depth must not be negative", ErrInvalidSnapshot)
	}
	if s.ParentID == "" && s.Depth != 0 {
		return fmt.Errorf("%w: root snapshot depth must be zero", ErrInvalidSnapshot)
	}
	if s.ParentID != "" && s.Depth == 0 {
		return fmt.Errorf("%w: child snapshot depth must be positive", ErrInvalidSnapshot)
	}
	if err := s.Deployment.Validate(); err != nil {
		return fmt.Errorf("%w: deployment: %w", ErrInvalidSnapshot, err)
	}
	if s.StartedAt.IsZero() || s.CapturedAt.IsZero() || s.CapturedAt.Before(s.StartedAt) {
		return fmt.Errorf("%w: started_at and captured_at must be ordered non-zero timestamps", ErrInvalidSnapshot)
	}
	if !s.Status.valid() {
		return fmt.Errorf("%w: unknown status %d", ErrInvalidSnapshot, s.Status)
	}
	if s.Status == StatusWaiting && s.Suspension == nil {
		return fmt.Errorf("%w: waiting snapshot requires suspension", ErrInvalidSnapshot)
	}
	if s.Status != StatusWaiting && s.Suspension != nil {
		return fmt.Errorf("%w: only waiting snapshot may carry suspension", ErrInvalidSnapshot)
	}
	if s.Status == StatusFailed && strings.TrimSpace(s.Failure) == "" {
		return fmt.Errorf("%w: failed snapshot requires failure", ErrInvalidSnapshot)
	}
	if s.Failure != "" && s.Status != StatusFailed && s.Status != StatusKilled {
		return fmt.Errorf("%w: only failed or killed snapshot may carry failure", ErrInvalidSnapshot)
	}
	if s.Suspension != nil {
		if err := s.Suspension.Validate(); err != nil {
			return fmt.Errorf("%w: suspension: %w", ErrInvalidSnapshot, err)
		}
	}
	if s.GoalName != strings.TrimSpace(s.GoalName) {
		return fmt.Errorf("%w: goal_name has surrounding whitespace", ErrInvalidSnapshot)
	}
	if math.IsNaN(s.OwnCost) || math.IsInf(s.OwnCost, 0) || s.OwnCost < 0 || s.OwnTokens < 0 {
		return fmt.Errorf("%w: own usage totals must be finite and non-negative", ErrInvalidSnapshot)
	}
	for i, run := range s.History {
		if strings.TrimSpace(run.ActionName) == "" || run.StartedAt.IsZero() || run.Duration < 0 || !run.Status.Valid() {
			return fmt.Errorf("%w: history[%d] is invalid", ErrInvalidSnapshot, i)
		}
	}
	for i, call := range s.OwnModelCalls {
		if err := call.Validate(); err != nil {
			return fmt.Errorf("%w: own_model_calls[%d]: %w", ErrInvalidSnapshot, i, err)
		}
	}
	for i, call := range s.OwnEmbeddingCalls {
		if err := call.Validate(); err != nil {
			return fmt.Errorf("%w: own_embedding_calls[%d]: %w", ErrInvalidSnapshot, i, err)
		}
	}
	for key, value := range s.Blackboard {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%w: blackboard has empty key", ErrInvalidSnapshot)
		}
		if err := value.Validate(); err != nil {
			return fmt.Errorf("%w: blackboard[%q]: %w", ErrInvalidSnapshot, key, err)
		}
	}
	for key := range s.Conditions {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%w: conditions has empty key", ErrInvalidSnapshot)
		}
	}
	for i, value := range s.Objects {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("%w: objects[%d]: %w", ErrInvalidSnapshot, i, err)
		}
	}
	return nil
}

func (s ProcessSnapshot) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(s.wire())
}

func (s *ProcessSnapshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSnapshot)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire processSnapshotWire
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidSnapshot, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON value", ErrInvalidSnapshot)
	}
	candidate, err := wire.snapshot()
	if err != nil {
		return err
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// ActionRunSnapshot is one durable action history row.
type ActionRunSnapshot struct {
	ActionName string        `json:"action"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration_ns"`
	Status     ActionStatus  `json:"-"`
}

type actionRunSnapshotWire struct {
	ActionName string        `json:"action"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration_ns"`
	Status     string        `json:"status"`
}

func (r ActionRunSnapshot) MarshalJSON() ([]byte, error) {
	if !r.Status.Valid() {
		return nil, fmt.Errorf("action run snapshot: unknown status %d", r.Status)
	}
	return json.Marshal(actionRunSnapshotWire{
		ActionName: r.ActionName,
		StartedAt:  r.StartedAt,
		Duration:   r.Duration,
		Status:     r.Status.String(),
	})
}

func (r *ActionRunSnapshot) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("action run snapshot: nil receiver")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire actionRunSnapshotWire
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("action run snapshot: decode: %w", err)
	}
	status, err := parseActionStatus(wire.Status)
	if err != nil {
		return fmt.Errorf("action run snapshot: %w", err)
	}
	*r = ActionRunSnapshot{
		ActionName: wire.ActionName,
		StartedAt:  wire.StartedAt,
		Duration:   wire.Duration,
		Status:     status,
	}
	return nil
}

// ProcessStore persists process snapshots for the runtime. Save receives one
// logical process-tree capture. Delete removes the durable tree rooted at the
// supplied process ID. Implementations own traversal, write coordination,
// transaction, and partial-failure behavior. Load returns [ErrSnapshotNotFound]
// for an unknown ID.
type ProcessStore interface {
	Save(context.Context, []ProcessSnapshot) error
	Load(context.Context, string) (ProcessSnapshot, error)
	Delete(context.Context, string) error
}

// ProcessLister is the optional administrative listing capability.
type ProcessLister interface {
	List(context.Context) ([]string, error)
}

func parseProcessStatus(status string) (ProcessStatus, error) {
	for _, candidate := range []ProcessStatus{StatusNotStarted, StatusRunning, StatusCompleted, StatusFailed, StatusStuck, StatusWaiting, StatusPaused, StatusTerminated, StatusKilled} {
		if status == candidate.String() {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("%w: unknown status %q", ErrInvalidSnapshot, status)
}
