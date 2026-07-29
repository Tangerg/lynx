package core_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func validSnapshot(id string) core.ProcessSnapshot {
	started := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	return core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "demo", Digest: "digest"},
		StartedAt:     started,
		Status:        core.StatusCompleted,
	}
}

// mutualParent builds a snapshot that names parent as its parent, so a test can
// assemble parent links the root never descends through.
func mutualParent(id, parent string) core.ProcessSnapshot {
	snapshot := validSnapshot(id)
	snapshot.ParentID = parent
	return snapshot
}

func TestProcessSnapshotRejectsUnknownAndMissingSchema(t *testing.T) {
	snapshot := validSnapshot("wire")
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["status"] != "completed" {
		t.Fatalf("status wire = %#v", decoded["status"])
	}
	if _, leaked := decoded["captured_at"]; leaked {
		t.Fatal("process snapshot leaked application-owned capture metadata")
	}
	var unsupported []uint16
	for version := uint16(0); version < core.ProcessSnapshotSchemaVersion; version++ {
		unsupported = append(unsupported, version)
	}
	unsupported = append(unsupported, core.ProcessSnapshotSchemaVersion+1)
	for _, version := range unsupported {
		decoded["schema_version"] = version
		invalid, _ := json.Marshal(decoded)
		var target core.ProcessSnapshot
		if err := json.Unmarshal(invalid, &target); !errors.Is(err, core.ErrSnapshotSchema) {
			t.Fatalf("schema %v error = %v", version, err)
		}
	}
}

func TestProcessSnapshotRejectsApplicationCaptureMetadata(t *testing.T) {
	var snapshot core.ProcessSnapshot
	err := json.Unmarshal(
		[]byte(fmt.Sprintf(
			`{"schema_version":%d,"id":"process","deployment":{"name":"demo","digest":"digest"},"started_at":"2026-07-16T08:00:00Z","captured_at":"2026-07-16T08:00:01Z","status":"completed","own_usage":{"cost":0,"tokens":0,"model_calls":0}}`,
			core.ProcessSnapshotSchemaVersion,
		)),
		&snapshot,
	)
	if !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("captured_at error = %v, want ErrInvalidSnapshot", err)
	}
}

func TestProcessSnapshotValidatesSpawnCallLineage(t *testing.T) {
	root := validSnapshot("root")
	root.SpawnCallID = "call-root"
	if err := root.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("root spawn call error = %v, want ErrInvalidSnapshot", err)
	}

	child := validSnapshot("child")
	child.ParentID = "root"
	child.SpawnCallID = " call-child "
	if err := child.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("whitespace spawn call error = %v, want ErrInvalidSnapshot", err)
	}

	child.SpawnCallID = "call-child"
	body, err := json.Marshal(child)
	if err != nil {
		t.Fatal(err)
	}
	var restored core.ProcessSnapshot
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ParentID != child.ParentID || restored.SpawnCallID != child.SpawnCallID {
		t.Fatalf("restored lineage = parent %q spawn %q, want parent %q spawn %q",
			restored.ParentID, restored.SpawnCallID, child.ParentID, child.SpawnCallID)
	}
}

func TestProcessSnapshotFailureHasExplicitWireShape(t *testing.T) {
	snapshot := validSnapshot("failed")
	snapshot.Status = core.StatusFailed
	snapshot.Failure = &core.ProcessFailure{Message: "provider unavailable"}

	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Failure *core.ProcessFailure `json:"failure"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Failure == nil || wire.Failure.Message != snapshot.Failure.Message {
		t.Fatalf("failure wire = %#v", wire.Failure)
	}

	var decoded core.ProcessSnapshot
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Failure == nil || decoded.Failure.Error() != snapshot.Failure.Error() {
		t.Fatalf("decoded failure = %#v", decoded.Failure)
	}

	unknownField := []byte(fmt.Sprintf(
		`{"schema_version":%d,"id":"failed","deployment":{"name":"demo","digest":"digest"},"started_at":"2026-07-16T08:00:00Z","status":"failed","failure":{"message":"failed","code":"magic"},"own_usage":{"cost":0,"tokens":0,"model_calls":0}}`,
		core.ProcessSnapshotSchemaVersion,
	))
	if err := json.Unmarshal(unknownField, &decoded); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("unknown failure field error = %v", err)
	}
}

func TestProcessSnapshotRejectsInvalidAggregate(t *testing.T) {
	for _, status := range []core.ProcessStatus{core.StatusNotStarted, core.StatusRunning} {
		unstable := validSnapshot("unstable")
		unstable.Status = status
		if err := unstable.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
			t.Fatalf("unstable status %s error = %v", status, err)
		}
	}
	invalid := validSnapshot("waiting")
	invalid.Status = core.StatusWaiting
	if err := invalid.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("waiting without suspension error = %v", err)
	}
	invalidUsage := validSnapshot("invalid-usage")
	invalidUsage.OwnUsage.Tokens = -1
	if err := invalidUsage.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("invalid usage error = %v", err)
	}
	failedWithoutCause := validSnapshot("failed-without-cause")
	failedWithoutCause.Status = core.StatusFailed
	if err := failedWithoutCause.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("failed without cause error = %v", err)
	}
	waitingWithFailure := validSnapshot("waiting-with-failure")
	waitingWithFailure.Status = core.StatusWaiting
	waitingWithFailure.Suspension = &interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            "approval",
		Prompt:        json.RawMessage(`"approve?"`), ResumeSchema: json.RawMessage(`{"type":"boolean"}`), CreatedAt: time.Now(),
	}
	waitingWithFailure.Failure = &core.ProcessFailure{Message: "must not survive"}
	if err := waitingWithFailure.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("waiting with failure error = %v", err)
	}
}

func TestProcessSnapshotTreeValidatesBoundary(t *testing.T) {
	root := validSnapshot("root")
	child := validSnapshot("child")
	child.ParentID = root.ID
	disconnected := child
	disconnected.ParentID = "outside"
	rootWithParent := root
	rootWithParent.ParentID = "outside"

	tests := []struct {
		name string
		tree core.ProcessSnapshotTree
	}{
		{name: "empty tree", tree: core.ProcessSnapshotTree{RootID: root.ID}},
		{name: "missing root", tree: core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{child},
		}},
		{name: "duplicate snapshot", tree: core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{root, root},
		}},
		{name: "root has external parent", tree: core.ProcessSnapshotTree{
			RootID: rootWithParent.ID, Snapshots: []core.ProcessSnapshot{rootWithParent},
		}},
		{name: "external parent", tree: core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{root, disconnected},
		}},
		{name: "self-parented process", tree: core.ProcessSnapshotTree{
			RootID:    root.ID,
			Snapshots: []core.ProcessSnapshot{root, mutualParent("a", "a")},
		}},
		// A self-parent is already rejected per snapshot. These two are not: each
		// process satisfies "my parent is in this capture" while sitting outside
		// the root's descent, and restore only walks down from the root, so
		// accepting them loses processes in silence.
		{name: "processes parenting each other", tree: core.ProcessSnapshotTree{
			RootID:    root.ID,
			Snapshots: []core.ProcessSnapshot{root, mutualParent("a", "b"), mutualParent("b", "a")},
		}},
		{name: "subtree hanging off a cycle", tree: core.ProcessSnapshotTree{
			RootID: root.ID,
			Snapshots: []core.ProcessSnapshot{
				root, mutualParent("a", "b"), mutualParent("b", "a"), mutualParent("c", "a"),
			},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.tree.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
				t.Fatalf("Validate error = %v, want ErrInvalidSnapshot", err)
			}
		})
	}

	valid := core.ProcessSnapshotTree{
		RootID: root.ID, Snapshots: []core.ProcessSnapshot{child, root},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid change: %v", err)
	}
}

func TestProcessSnapshotTreeRejectsAggregateUsageOverflow(t *testing.T) {
	for name, usage := range map[string]core.Usage{
		"cost":        {Cost: math.MaxFloat64},
		"tokens":      {Tokens: math.MaxInt64},
		"model calls": {ModelCalls: math.MaxInt},
		"actions":     {Actions: math.MaxInt},
	} {
		t.Run(name, func(t *testing.T) {
			root := validSnapshot("root")
			root.OwnUsage = usage
			child := validSnapshot("child")
			child.ParentID = root.ID
			child.OwnUsage = core.Usage{Cost: 1, Tokens: 1, ModelCalls: 1, Actions: 1}
			tree := core.ProcessSnapshotTree{
				RootID:    root.ID,
				Snapshots: []core.ProcessSnapshot{root, child},
			}
			if err := tree.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
				t.Fatalf("Validate error = %v, want ErrInvalidSnapshot", err)
			}
		})
	}
}

// TestProcessSnapshotTreeReportsRootAndUsage pins the two facts a consumer would
// otherwise re-derive from the tree's fields: which snapshot is the root, and
// what the capture consumed in total.
func TestProcessSnapshotTreeReportsRootAndUsage(t *testing.T) {
	root := validSnapshot("root")
	root.OwnUsage = core.Usage{Cost: 1.5, Tokens: 10, ModelCalls: 2, Actions: 3}
	child := validSnapshot("child")
	child.ParentID = root.ID
	child.OwnUsage = core.Usage{Cost: 0.5, Tokens: 4, ModelCalls: 1, Actions: 1}
	tree := core.ProcessSnapshotTree{RootID: root.ID, Snapshots: []core.ProcessSnapshot{child, root}}
	if err := tree.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	got, ok := tree.Root()
	if !ok || got.ID != root.ID {
		t.Fatalf("Root() = %q (present=%t), want %q", got.ID, ok, root.ID)
	}
	usage, err := tree.Usage()
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage != (core.Usage{Cost: 2, Tokens: 14, ModelCalls: 3, Actions: 4}) {
		t.Fatalf("Usage() = %+v, want the sum of both processes", usage)
	}

	if _, ok := (core.ProcessSnapshotTree{RootID: "absent", Snapshots: []core.ProcessSnapshot{child}}).Root(); ok {
		t.Fatal("Root() found a root in a tree that has none")
	}
}
