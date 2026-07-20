package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ProcessSnapshotSchemaVersion is the only durable process wire schema this
// development version accepts. Missing and unknown versions fail explicitly;
// the framework never guesses an obsolete snapshot shape.
const ProcessSnapshotSchemaVersion uint16 = 2

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
		if strings.TrimSpace(run.ActionName) == "" || run.StartedAt.IsZero() || run.Duration < 0 || run.Attempts < 1 || !validActionStatusString(run.Status) {
			return fmt.Errorf("%w: history[%d] is invalid", ErrInvalidSnapshot, i)
		}
	}
	for i, call := range s.OwnModelCalls {
		if !call.valid() {
			return fmt.Errorf("%w: own_model_calls[%d] is invalid", ErrInvalidSnapshot, i)
		}
	}
	for i, call := range s.OwnEmbeddingCalls {
		if !call.valid() {
			return fmt.Errorf("%w: own_embedding_calls[%d] is invalid", ErrInvalidSnapshot, i)
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

// MemoryProcessStore is the strict reference ProcessStore. It applies the
// same JSON boundary and CAS semantics expected from persistent backends.
type MemoryProcessStore struct {
	mu        sync.RWMutex
	snapshots map[string]ProcessSnapshot
}

// NewMemoryProcessStore returns an empty process store.
func NewMemoryProcessStore() *MemoryProcessStore {
	return &MemoryProcessStore{snapshots: map[string]ProcessSnapshot{}}
}

func (s *MemoryProcessStore) Save(_ context.Context, snapshot ProcessSnapshot, expectedRevision uint64) (uint64, error) {
	if s == nil {
		return 0, errors.New("memory process store: nil receiver")
	}
	if snapshot.Revision != expectedRevision {
		return 0, fmt.Errorf("%w: snapshot revision %d does not match expected revision %d", ErrInvalidSnapshot, snapshot.Revision, expectedRevision)
	}
	if err := snapshot.Validate(); err != nil {
		return 0, fmt.Errorf("memory process store: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	actualRevision := uint64(0)
	if current, ok := s.snapshots[snapshot.ID]; ok {
		actualRevision = current.Revision
	}
	if actualRevision != expectedRevision {
		return 0, &RevisionConflictError{ProcessID: snapshot.ID, Expected: expectedRevision, Actual: actualRevision}
	}
	snapshot.Revision = actualRevision + 1
	cloned, err := snapshot.clone()
	if err != nil {
		return 0, fmt.Errorf("memory process store: clone snapshot: %w", err)
	}
	s.snapshots[snapshot.ID] = cloned
	return snapshot.Revision, nil
}

func (s *MemoryProcessStore) Load(_ context.Context, id string) (ProcessSnapshot, error) {
	if s == nil {
		return ProcessSnapshot{}, errors.New("memory process store: nil receiver")
	}
	s.mu.RLock()
	snapshot, ok := s.snapshots[id]
	s.mu.RUnlock()
	if !ok {
		return ProcessSnapshot{}, fmt.Errorf("memory process store: load %q: %w", id, ErrSnapshotNotFound)
	}
	cloned, err := snapshot.clone()
	if err != nil {
		return ProcessSnapshot{}, fmt.Errorf("memory process store: clone loaded snapshot: %w", err)
	}
	if cloned.Revision == 0 {
		return ProcessSnapshot{}, fmt.Errorf("%w: stored revision is zero", ErrInvalidSnapshot)
	}
	return cloned, nil
}

func (s *MemoryProcessStore) Delete(_ context.Context, id string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	delete(s.snapshots, id)
	s.mu.Unlock()
	return nil
}

func (s *MemoryProcessStore) List(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	ids := make([]string, 0, len(s.snapshots))
	for id := range s.snapshots {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	slices.Sort(ids)
	return ids, nil
}

func (s ProcessSnapshot) clone() (ProcessSnapshot, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	var cloned ProcessSnapshot
	if err := json.Unmarshal(data, &cloned); err != nil {
		return ProcessSnapshot{}, err
	}
	return cloned, nil
}
