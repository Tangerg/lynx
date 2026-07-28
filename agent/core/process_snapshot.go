package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ProcessSnapshotSchemaVersion is the only portable process wire schema this
// development version accepts. Missing and unknown versions fail explicitly;
// the framework never guesses an obsolete snapshot shape.
const ProcessSnapshotSchemaVersion uint16 = 12

var (
	ErrSnapshotSchema  = errors.New("process snapshot: unsupported schema")
	ErrInvalidSnapshot = errors.New("process snapshot: invalid")
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

// ProcessSnapshot is the portable execution state owned by the framework for
// one process. Delegation depth is not stored: it follows from the parent links
// inside the capture, and a second copy would only be a fact to keep in
// agreement. Restoring also requires ProcessOptions to reattach the host's
// live capabilities and execution policy. OwnUsage contains only this
// process's direct execution-resource counters; descendants carry their own
// snapshots, and runtime reconstructs aggregate usage through parent-child
// links. Runtime-only objects, derived world state, functions, and closures
// are intentionally absent.
type ProcessSnapshot struct {
	SchemaVersion uint16 `json:"schema_version"`

	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`

	Deployment DeploymentRef `json:"deployment"`
	StartedAt  time.Time     `json:"started_at"`
	Status     ProcessStatus `json:"-"`

	Suspension *interaction.Suspension `json:"suspension,omitempty"`
	GoalName   string                  `json:"goal_name,omitempty"`
	Failure    *ProcessFailure         `json:"failure,omitempty"`

	OwnUsage Usage `json:"own_usage"`

	Blackboard map[string]TaggedValue `json:"blackboard,omitempty"`
	Conditions map[string]bool        `json:"conditions,omitempty"`
	Objects    []TaggedValue          `json:"objects,omitempty"`
}

type processSnapshotWire struct {
	SchemaVersion uint16                  `json:"schema_version"`
	ID            string                  `json:"id"`
	ParentID      string                  `json:"parent_id,omitempty"`
	Deployment    DeploymentRef           `json:"deployment"`
	StartedAt     time.Time               `json:"started_at"`
	Status        string                  `json:"status"`
	Suspension    *interaction.Suspension `json:"suspension,omitempty"`
	GoalName      string                  `json:"goal_name,omitempty"`
	Failure       *ProcessFailure         `json:"failure,omitempty"`
	OwnUsage      Usage                   `json:"own_usage"`
	Blackboard    map[string]TaggedValue  `json:"blackboard,omitempty"`
	Conditions    map[string]bool         `json:"conditions,omitempty"`
	Objects       []TaggedValue           `json:"objects,omitempty"`
}

func (s ProcessSnapshot) wire() processSnapshotWire {
	return processSnapshotWire{
		SchemaVersion: s.SchemaVersion,
		ID:            s.ID, ParentID: s.ParentID,
		Deployment: s.Deployment, StartedAt: s.StartedAt,
		Status: s.Status.String(), Suspension: s.Suspension, GoalName: s.GoalName,
		Failure: s.Failure, OwnUsage: s.OwnUsage,
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
		ID:            w.ID, ParentID: w.ParentID,
		Deployment: w.Deployment, StartedAt: w.StartedAt,
		Status: status, Suspension: w.Suspension, GoalName: w.GoalName,
		Failure: w.Failure, OwnUsage: w.OwnUsage,
		Blackboard: w.Blackboard, Conditions: w.Conditions, Objects: w.Objects,
	}, nil
}

// Validate checks the portable process state without mutating it.
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
	if err := s.Deployment.Validate(); err != nil {
		return fmt.Errorf("%w: deployment: %w", ErrInvalidSnapshot, err)
	}
	if s.StartedAt.IsZero() {
		return fmt.Errorf("%w: started_at must be non-zero", ErrInvalidSnapshot)
	}
	if !s.Status.valid() {
		return fmt.Errorf("%w: unknown status %d", ErrInvalidSnapshot, s.Status)
	}
	if !s.Status.snapshotStable() {
		return fmt.Errorf("%w: status %s is not a stable checkpoint", ErrInvalidSnapshot, s.Status)
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
	if err := s.OwnUsage.Validate(); err != nil {
		return fmt.Errorf("%w: own_usage: %w", ErrInvalidSnapshot, err)
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

// ProcessSnapshotTree is one complete runtime capture rooted at RootID.
// Snapshots are unordered; parent links define the tree. A capture always owns
// the complete process tree, so its root has no parent and depth zero.
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
	if _, err := t.Usage(); err != nil {
		return err
	}

	root, ok := byID[t.RootID]
	if !ok {
		return fmt.Errorf("%w: tree root %q is missing", ErrInvalidSnapshot, t.RootID)
	}
	if root.ParentID != "" {
		return fmt.Errorf("%w: tree root %q must have no parent", ErrInvalidSnapshot, root.ID)
	}

	children := make(map[string][]string, len(t.Snapshots))
	for _, snapshot := range t.Snapshots {
		if snapshot.ID == t.RootID {
			continue
		}
		if _, found := byID[snapshot.ParentID]; !found {
			return fmt.Errorf("%w: process %q has parent %q outside tree rooted at %q", ErrInvalidSnapshot, snapshot.ID, snapshot.ParentID, t.RootID)
		}
		children[snapshot.ParentID] = append(children[snapshot.ParentID], snapshot.ID)
	}

	// Present parent links do not make one tree: two processes naming each other
	// each satisfy "my parent is here" while forming a cycle the root never
	// reaches. Restore descends from the root, so an unreachable snapshot would
	// be dropped without a word. Requiring that the descent reaches every
	// snapshot is what proves connected, acyclic, and rooted at once — the
	// property a capture claims by carrying a RootID at all.
	//
	// The walk terminates regardless: each snapshot lists exactly one parent, so
	// it is enqueued at most once, and the root belongs to no child list.
	reached := 1
	pending := []string{t.RootID}
	for len(pending) > 0 {
		parent := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, child := range children[parent] {
			reached++
			pending = append(pending, child)
		}
	}
	if reached != len(t.Snapshots) {
		return fmt.Errorf(
			"%w: %d of %d processes are unreachable from tree root %q",
			ErrInvalidSnapshot,
			len(t.Snapshots)-reached,
			len(t.Snapshots),
			t.RootID,
		)
	}
	return nil
}

// Root returns the snapshot the tree is rooted at. A tree that passed Validate
// always has one; ok is false only for a tree that has not been checked.
func (t ProcessSnapshotTree) Root() (ProcessSnapshot, bool) {
	for _, snapshot := range t.Snapshots {
		if snapshot.ID == t.RootID {
			return snapshot, true
		}
	}
	return ProcessSnapshot{}, false
}

// Usage reports what the captured tree consumed: the sum of every process's
// direct counters, which is the same total the live tree reports through
// ProcessView. It errors when the sum would exceed runtime capacity, and
// Validate rejects such a tree, so a validated tree's total is exact.
func (t ProcessSnapshotTree) Usage() (Usage, error) {
	var total Usage
	for _, snapshot := range t.Snapshots {
		if snapshot.OwnUsage.Cost > math.MaxFloat64-total.Cost ||
			snapshot.OwnUsage.Tokens > math.MaxInt64-total.Tokens ||
			snapshot.OwnUsage.ModelCalls > math.MaxInt-total.ModelCalls ||
			snapshot.OwnUsage.Actions > math.MaxInt-total.Actions {
			return Usage{}, fmt.Errorf("%w: process tree usage exceeds runtime capacity", ErrInvalidSnapshot)
		}
		total.Cost += snapshot.OwnUsage.Cost
		total.Tokens += snapshot.OwnUsage.Tokens
		total.ModelCalls += snapshot.OwnUsage.ModelCalls
		total.Actions += snapshot.OwnUsage.Actions
	}
	return total, nil
}

func parseProcessStatus(status string) (ProcessStatus, error) {
	for _, candidate := range []ProcessStatus{StatusNotStarted, StatusRunning, StatusCompleted, StatusFailed, StatusStuck, StatusWaiting, StatusPaused, StatusTerminated, StatusKilled} {
		if status == candidate.String() {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("%w: unknown status %q", ErrInvalidSnapshot, status)
}
