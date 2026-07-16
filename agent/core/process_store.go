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
const ProcessSnapshotSchemaVersion uint16 = 1

var (
	ErrSnapshotNotFound = errors.New("process store: snapshot not found")
	ErrSnapshotSchema   = errors.New("process snapshot: unsupported schema")
	ErrInvalidSnapshot  = errors.New("process snapshot: invalid")
	ErrRevisionConflict = errors.New("process store: revision conflict")
)

// RevisionConflictError reports the compare-and-swap values that prevented a
// snapshot write. It matches [ErrRevisionConflict].
type RevisionConflictError struct {
	ProcessID string
	Expected  uint64
	Actual    uint64
}

func (e *RevisionConflictError) Error() string {
	if e == nil {
		return ErrRevisionConflict.Error()
	}
	return fmt.Sprintf("%s for process %q: expected %d, actual %d", ErrRevisionConflict, e.ProcessID, e.Expected, e.Actual)
}

func (e *RevisionConflictError) Unwrap() error { return ErrRevisionConflict }

// ProcessSnapshot is the complete durable state required to inspect or resume
// one process. Runtime-only objects, derived world state, functions, and
// closures are intentionally absent.
type ProcessSnapshot struct {
	SchemaVersion uint16 `json:"schema_version"`
	Revision      uint64 `json:"revision"`

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

	Cost   float64 `json:"cost"`
	Tokens int     `json:"tokens"`

	ModelCalls     []ModelCall     `json:"model_calls,omitempty"`
	EmbeddingCalls []EmbeddingCall `json:"embedding_calls,omitempty"`

	Blackboard map[string]TaggedValue `json:"blackboard,omitempty"`
	Conditions map[string]bool        `json:"conditions,omitempty"`
	Objects    []TaggedValue          `json:"objects,omitempty"`
}

type processSnapshotWire struct {
	SchemaVersion  uint16                  `json:"schema_version"`
	Revision       uint64                  `json:"revision"`
	ID             string                  `json:"id"`
	ParentID       string                  `json:"parent_id,omitempty"`
	Depth          int                     `json:"depth,omitempty"`
	Deployment     DeploymentRef           `json:"deployment"`
	StartedAt      time.Time               `json:"started_at"`
	CapturedAt     time.Time               `json:"captured_at"`
	Status         string                  `json:"status"`
	Suspension     *interaction.Suspension `json:"suspension,omitempty"`
	GoalName       string                  `json:"goal_name,omitempty"`
	History        []ActionRunSnapshot     `json:"history,omitempty"`
	Failure        string                  `json:"failure,omitempty"`
	Cost           float64                 `json:"cost"`
	Tokens         int                     `json:"tokens"`
	ModelCalls     []ModelCall             `json:"model_calls,omitempty"`
	EmbeddingCalls []EmbeddingCall         `json:"embedding_calls,omitempty"`
	Blackboard     map[string]TaggedValue  `json:"blackboard,omitempty"`
	Conditions     map[string]bool         `json:"conditions,omitempty"`
	Objects        []TaggedValue           `json:"objects,omitempty"`
}

func (s ProcessSnapshot) wire() processSnapshotWire {
	return processSnapshotWire{
		SchemaVersion: s.SchemaVersion, Revision: s.Revision,
		ID: s.ID, ParentID: s.ParentID, Depth: s.Depth,
		Deployment: s.Deployment, StartedAt: s.StartedAt, CapturedAt: s.CapturedAt,
		Status: s.Status.String(), Suspension: s.Suspension, GoalName: s.GoalName,
		History: s.History, Failure: s.Failure, Cost: s.Cost, Tokens: s.Tokens,
		ModelCalls: s.ModelCalls, EmbeddingCalls: s.EmbeddingCalls,
		Blackboard: s.Blackboard, Conditions: s.Conditions, Objects: s.Objects,
	}
}

func processSnapshotFromWire(wire processSnapshotWire) (ProcessSnapshot, error) {
	status, err := parseProcessStatus(wire.Status)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	return ProcessSnapshot{
		SchemaVersion: wire.SchemaVersion, Revision: wire.Revision,
		ID: wire.ID, ParentID: wire.ParentID, Depth: wire.Depth,
		Deployment: wire.Deployment, StartedAt: wire.StartedAt, CapturedAt: wire.CapturedAt,
		Status: status, Suspension: wire.Suspension, GoalName: wire.GoalName,
		History: wire.History, Failure: wire.Failure, Cost: wire.Cost, Tokens: wire.Tokens,
		ModelCalls: wire.ModelCalls, EmbeddingCalls: wire.EmbeddingCalls,
		Blackboard: wire.Blackboard, Conditions: wire.Conditions, Objects: wire.Objects,
	}, nil
}

// Validate checks the durable aggregate without mutating it.
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
	if err := s.Deployment.Validate(); err != nil {
		return fmt.Errorf("%w: deployment: %w", ErrInvalidSnapshot, err)
	}
	if s.StartedAt.IsZero() || s.CapturedAt.IsZero() || s.CapturedAt.Before(s.StartedAt) {
		return fmt.Errorf("%w: started_at and captured_at must be ordered non-zero timestamps", ErrInvalidSnapshot)
	}
	if !validProcessStatus(s.Status) {
		return fmt.Errorf("%w: unknown status %d", ErrInvalidSnapshot, s.Status)
	}
	if s.Status == StatusWaiting && s.Suspension == nil {
		return fmt.Errorf("%w: waiting snapshot requires suspension", ErrInvalidSnapshot)
	}
	if s.Status != StatusWaiting && s.Suspension != nil {
		return fmt.Errorf("%w: only waiting snapshot may carry suspension", ErrInvalidSnapshot)
	}
	if s.Suspension != nil {
		if err := s.Suspension.Validate(); err != nil {
			return fmt.Errorf("%w: suspension: %w", ErrInvalidSnapshot, err)
		}
	}
	if s.GoalName != strings.TrimSpace(s.GoalName) {
		return fmt.Errorf("%w: goal_name has surrounding whitespace", ErrInvalidSnapshot)
	}
	if math.IsNaN(s.Cost) || math.IsInf(s.Cost, 0) || s.Cost < 0 || s.Tokens < 0 {
		return fmt.Errorf("%w: usage totals must be finite and non-negative", ErrInvalidSnapshot)
	}
	for i, run := range s.History {
		if strings.TrimSpace(run.ActionName) == "" || run.StartedAt.IsZero() || run.Duration < 0 || run.Attempts < 1 || !validActionStatusString(run.Status) {
			return fmt.Errorf("%w: history[%d] is invalid", ErrInvalidSnapshot, i)
		}
	}
	for i, call := range s.ModelCalls {
		if !validModelCall(call) {
			return fmt.Errorf("%w: model_calls[%d] is invalid", ErrInvalidSnapshot, i)
		}
	}
	for i, call := range s.EmbeddingCalls {
		if !validEmbeddingCall(call) {
			return fmt.Errorf("%w: embedding_calls[%d] is invalid", ErrInvalidSnapshot, i)
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
	candidate, err := processSnapshotFromWire(wire)
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
	Status     string        `json:"status"`
	Attempts   int           `json:"attempts"`
}

// SnapshotReader loads the latest committed snapshot revision.
type SnapshotReader interface {
	Load(context.Context, string) (ProcessSnapshot, error)
}

// SnapshotWriter atomically writes snapshot only when expectedRevision is the
// currently stored revision. A new process uses expectedRevision 0 and commits
// revision 1.
type SnapshotWriter interface {
	Save(context.Context, ProcessSnapshot, uint64) (uint64, error)
}

// ProcessStore is the minimum persistence capability required by Engine.
// CAS prevents lost snapshot updates; it does not grant execution ownership.
// The framework assumes one active owner drives a process at a time. Hosts
// that hand processes across nodes must add a lease or fencing protocol before
// allowing another worker to execute the same process.
type ProcessStore interface {
	SnapshotReader
	SnapshotWriter
}

// SnapshotDeleter is the optional idempotent cleanup capability.
type SnapshotDeleter interface {
	Delete(context.Context, string) error
}

// SnapshotLister is the optional administrative listing capability.
type SnapshotLister interface {
	List(context.Context) ([]string, error)
}

func validProcessStatus(status ProcessStatus) bool {
	switch status {
	case StatusNotStarted, StatusRunning, StatusCompleted, StatusFailed, StatusStuck, StatusWaiting, StatusPaused, StatusTerminated, StatusKilled:
		return true
	default:
		return false
	}
}

func parseProcessStatus(status string) (ProcessStatus, error) {
	for _, candidate := range []ProcessStatus{StatusNotStarted, StatusRunning, StatusCompleted, StatusFailed, StatusStuck, StatusWaiting, StatusPaused, StatusTerminated, StatusKilled} {
		if status == candidate.String() {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("%w: unknown status %q", ErrInvalidSnapshot, status)
}

func validActionStatusString(status string) bool {
	return status == ActionSucceeded.String() || status == ActionFailed.String() || status == ActionWaiting.String() || status == ActionPaused.String()
}

func validModelCall(call ModelCall) bool {
	return !call.Timestamp.IsZero() &&
		!math.IsNaN(call.CostUSD) && !math.IsInf(call.CostUSD, 0) && call.CostUSD >= 0 &&
		call.PromptTokens >= 0 && call.CompletionTokens >= 0 && call.ReasoningTokens >= 0 &&
		call.CacheReadInputTokens >= 0 && call.CacheWriteInputTokens >= 0 &&
		call.ReasoningTokens <= call.CompletionTokens && call.Duration >= 0
}

func validEmbeddingCall(call EmbeddingCall) bool {
	return !call.Timestamp.IsZero() &&
		!math.IsNaN(call.CostUSD) && !math.IsInf(call.CostUSD, 0) && call.CostUSD >= 0 &&
		call.InputTokens >= 0 && call.InputCount >= 0 && call.Duration >= 0
}
