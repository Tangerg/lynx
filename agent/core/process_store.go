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
const ProcessSnapshotSchemaVersion uint16 = 5

var (
	ErrSnapshotNotFound = errors.New("process store: snapshot not found")
	ErrSnapshotSchema   = errors.New("process snapshot: unsupported schema")
	ErrInvalidSnapshot  = errors.New("process snapshot: invalid")
)

// ProcessFailure is the portable failure representation stored in a process
// snapshot. A live Go error may carry sentinel identity, an unwrap chain, and
// implementation-specific fields that have no general wire representation;
// snapshots therefore preserve only its human-readable message. After restore,
// the process failure accessor returns a *ProcessFailure so callers can
// distinguish this documented message-only value with [errors.As].
type ProcessFailure struct {
	Message string `json:"message"`
}

// Error implements error.
func (f *ProcessFailure) Error() string {
	if f == nil {
		return ""
	}
	return f.Message
}

func (f ProcessFailure) validate() error {
	if strings.TrimSpace(f.Message) == "" {
		return fmt.Errorf("%w: failure message must not be empty", ErrInvalidSnapshot)
	}
	return nil
}

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
	Failure    *ProcessFailure         `json:"failure,omitempty"`

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
	Failure           *ProcessFailure         `json:"failure,omitempty"`
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
	if s.Status == StatusFailed && s.Failure == nil {
		return fmt.Errorf("%w: failed snapshot requires failure", ErrInvalidSnapshot)
	}
	if s.Failure != nil && s.Status != StatusFailed && s.Status != StatusKilled {
		return fmt.Errorf("%w: only failed or killed snapshot may carry failure", ErrInvalidSnapshot)
	}
	if s.Failure != nil {
		if err := s.Failure.validate(); err != nil {
			return err
		}
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

// ProcessSnapshotTree is one complete runtime capture rooted at RootID.
// Snapshots are unordered; parent links define the tree. The root may itself
// have a parent outside the capture when a child subtree is saved directly.
type ProcessSnapshotTree struct {
	RootID    string
	Snapshots []ProcessSnapshot
}

// Validate checks that the capture is one connected process tree.
func (t ProcessSnapshotTree) Validate() error {
	if strings.TrimSpace(t.RootID) == "" || strings.TrimSpace(t.RootID) != t.RootID {
		return fmt.Errorf("%w: tree root ID must be non-empty without surrounding whitespace", ErrInvalidSnapshot)
	}
	if len(t.Snapshots) == 0 {
		return fmt.Errorf("%w: process tree is empty", ErrInvalidSnapshot)
	}

	byID := make(map[string]ProcessSnapshot, len(t.Snapshots))
	for index, snapshot := range t.Snapshots {
		if err := snapshot.Validate(); err != nil {
			return fmt.Errorf("process snapshot tree: snapshots[%d]: %w", index, err)
		}
		if _, duplicate := byID[snapshot.ID]; duplicate {
			return fmt.Errorf("%w: duplicate process ID %q", ErrInvalidSnapshot, snapshot.ID)
		}
		byID[snapshot.ID] = snapshot
	}

	root, ok := byID[t.RootID]
	if !ok {
		return fmt.Errorf("%w: tree root %q is missing", ErrInvalidSnapshot, t.RootID)
	}
	if _, parentInsideTree := byID[root.ParentID]; parentInsideTree {
		return fmt.Errorf("%w: root %q has parent %q inside its own tree", ErrInvalidSnapshot, root.ID, root.ParentID)
	}

	for _, snapshot := range t.Snapshots {
		if snapshot.ID == t.RootID {
			continue
		}
		parent, found := byID[snapshot.ParentID]
		if !found {
			return fmt.Errorf("%w: process %q has parent %q outside tree rooted at %q", ErrInvalidSnapshot, snapshot.ID, snapshot.ParentID, t.RootID)
		}
		if snapshot.Depth != parent.Depth+1 {
			return fmt.Errorf("%w: process %q depth %d does not follow parent %q depth %d", ErrInvalidSnapshot, snapshot.ID, snapshot.Depth, parent.ID, parent.Depth)
		}
	}
	return nil
}

// ProcessSnapshotChange is one logical persistence operation. Tree replaces
// the captured process state when non-nil; DeleteRoots removes obsolete durable
// trees. A store implementation chooses its own coordination, transaction, and
// partial-failure behavior, but a nil error means every requested change was
// applied. Implementations must not retain caller-owned slices or maps.
type ProcessSnapshotChange struct {
	Tree        *ProcessSnapshotTree
	DeleteRoots []string
}

// Validate checks the complete persistence operation.
func (c ProcessSnapshotChange) Validate() error {
	if c.Tree == nil && len(c.DeleteRoots) == 0 {
		return fmt.Errorf("%w: process snapshot change is empty", ErrInvalidSnapshot)
	}

	saved := make(map[string]struct{})
	if c.Tree != nil {
		if err := c.Tree.Validate(); err != nil {
			return err
		}
		saved = make(map[string]struct{}, len(c.Tree.Snapshots))
		for _, snapshot := range c.Tree.Snapshots {
			saved[snapshot.ID] = struct{}{}
		}
	}

	deletes := make(map[string]struct{}, len(c.DeleteRoots))
	for index, rootID := range c.DeleteRoots {
		if strings.TrimSpace(rootID) == "" || strings.TrimSpace(rootID) != rootID {
			return fmt.Errorf("%w: delete_roots[%d] must be non-empty without surrounding whitespace", ErrInvalidSnapshot, index)
		}
		if _, duplicate := deletes[rootID]; duplicate {
			return fmt.Errorf("%w: duplicate delete root %q", ErrInvalidSnapshot, rootID)
		}
		if _, conflict := saved[rootID]; conflict {
			return fmt.Errorf("%w: process %q cannot be saved and deleted in one change", ErrInvalidSnapshot, rootID)
		}
		deletes[rootID] = struct{}{}
	}
	return nil
}

// ProcessStore persists process snapshots for the runtime. Implementations must
// be safe for concurrent use and return ownership-isolated values. Apply
// receives each complete logical persistence change in one call so the
// implementation, not the framework, owns storage coordination. Load returns
// [ErrSnapshotNotFound] for an unknown ID.
type ProcessStore interface {
	Apply(context.Context, ProcessSnapshotChange) error
	Load(context.Context, string) (ProcessSnapshot, error)
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
