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
const ProcessSnapshotSchemaVersion uint16 = 3

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
// one process. OwnCost, OwnTokens, OwnModelCalls, and OwnEmbeddingCalls contain
// only this process's direct ledger; descendants persist their own snapshots,
// and runtime reconstructs aggregate usage through parent-child links.
// Runtime-only objects, derived world state, functions, and closures are
// intentionally absent.
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
	Revision          uint64                  `json:"revision"`
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
		SchemaVersion: s.SchemaVersion, Revision: s.Revision,
		ID: s.ID, ParentID: s.ParentID, Depth: s.Depth,
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
		SchemaVersion: w.SchemaVersion, Revision: w.Revision,
		ID: w.ID, ParentID: w.ParentID, Depth: w.Depth,
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
		if strings.TrimSpace(run.ActionName) == "" || run.StartedAt.IsZero() || run.Duration < 0 || run.Attempts < 1 || !run.Status.Valid() {
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
	Attempts   int           `json:"attempts"`
}

type actionRunSnapshotWire struct {
	ActionName string        `json:"action"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration_ns"`
	Status     string        `json:"status"`
	Attempts   int           `json:"attempts"`
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
		Attempts:   r.Attempts,
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
		Attempts:   wire.Attempts,
	}
	return nil
}

// SnapshotMutation is one atomic durable change set. Writes use Revision as a
// compare-and-swap expectation: revision zero creates a process, and every
// successful write stores Revision+1. Each DeleteTrees entry idempotently
// deletes that root and every durable descendant. Implementations reject a
// write whose lineage enters a deleted tree. A process ID may occur exactly
// once across the mutation roots and writes.
type SnapshotMutation struct {
	Writes      []ProcessSnapshot
	DeleteTrees []string
}

// Validate checks mutation shape and every snapshot without consulting a
// store. An empty mutation is a valid no-op.
func (m SnapshotMutation) Validate() error {
	seen := make(map[string]string, len(m.Writes)+len(m.DeleteTrees))
	pending := make(map[string]ProcessSnapshot, len(m.Writes))
	for index, snapshot := range m.Writes {
		if snapshot.Revision == math.MaxUint64 {
			return fmt.Errorf("%w: writes[%d] process %q revision is exhausted", ErrInvalidSnapshot, index, snapshot.ID)
		}
		if err := snapshot.Validate(); err != nil {
			return fmt.Errorf("writes[%d]: %w", index, err)
		}
		if previous, duplicate := seen[snapshot.ID]; duplicate {
			return fmt.Errorf("%w: process ID %q occurs in %s and writes[%d]", ErrInvalidSnapshot, snapshot.ID, previous, index)
		}
		seen[snapshot.ID] = fmt.Sprintf("writes[%d]", index)
		pending[snapshot.ID] = snapshot
	}
	deleteRoots := make(map[string]struct{}, len(m.DeleteTrees))
	for index, id := range m.DeleteTrees {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return fmt.Errorf("%w: delete_trees[%d] must be non-empty without surrounding whitespace", ErrInvalidSnapshot, index)
		}
		if previous, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: process ID %q occurs in %s and delete_trees[%d]", ErrInvalidSnapshot, id, previous, index)
		}
		seen[id] = fmt.Sprintf("delete_trees[%d]", index)
		deleteRoots[id] = struct{}{}
	}
	for index, snapshot := range m.Writes {
		visited := map[string]struct{}{snapshot.ID: {}}
		for parentID := snapshot.ParentID; parentID != ""; {
			if _, deleted := deleteRoots[parentID]; deleted {
				return fmt.Errorf("%w: writes[%d] process %q descends from deleted tree %q", ErrInvalidSnapshot, index, snapshot.ID, parentID)
			}
			if _, duplicate := visited[parentID]; duplicate {
				return fmt.Errorf("%w: writes[%d] process %q has cyclic pending lineage", ErrInvalidSnapshot, index, snapshot.ID)
			}
			visited[parentID] = struct{}{}
			parent, exists := pending[parentID]
			if !exists {
				break
			}
			parentID = parent.ParentID
		}
	}
	return nil
}

// ProcessStore owns durable process snapshots. Apply validates every write and
// compare-and-swap expectation before changing storage, then commits all writes
// and tree deletions atomically. An error leaves the entire mutation unapplied.
// Load returns [ErrSnapshotNotFound] for an unknown ID. List returns every
// stored ID; callers that need ordering must sort the result themselves.
//
// CAS prevents lost snapshot updates; it does not grant execution ownership.
// Hosts that hand processes across nodes must add a lease or fencing protocol
// before another worker executes the same process.
type ProcessStore interface {
	Apply(context.Context, SnapshotMutation) error
	Load(context.Context, string) (ProcessSnapshot, error)
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
