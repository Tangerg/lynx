package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/storetest"
)

func validSnapshot(id string) core.ProcessSnapshot {
	started := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	return core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "demo", Digest: "digest"},
		StartedAt:     started,
		CapturedAt:    started.Add(time.Second),
		Status:        core.StatusRunning,
	}
}

func validSnapshotChange(snapshots ...core.ProcessSnapshot) core.ProcessSnapshotChange {
	return core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
		RootID:    snapshots[0].ID,
		Snapshots: snapshots,
	}}
}

func TestMemoryProcessStoreReplacementAndDefensiveLoad(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	ctx := context.Background()
	snapshot := validSnapshot("p-1")
	snapshot.OwnTokens = 1500

	err := store.Apply(ctx, validSnapshotChange(snapshot))
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	loaded, err := store.Load(ctx, snapshot.ID)
	if err != nil || loaded.OwnTokens != 1500 {
		t.Fatalf("Load = %#v, err %v", loaded, err)
	}
	loaded.OwnTokens = 99
	again, _ := store.Load(ctx, snapshot.ID)
	if again.OwnTokens != 1500 {
		t.Fatal("Load returned mutable stored state")
	}

	snapshot.OwnTokens = 2000
	if err := store.Apply(ctx, validSnapshotChange(snapshot)); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	updated, err := store.Load(ctx, snapshot.ID)
	if err != nil || updated.OwnTokens != 2000 {
		t.Fatalf("updated Load = %#v, err %v", updated, err)
	}
}

func TestMemoryProcessStoreManagementCapabilities(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	ctx := context.Background()
	for _, id := range []string{"c", "a", "b"} {
		if err := store.Apply(ctx, validSnapshotChange(validSnapshot(id))); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil || len(ids) != 3 || ids[0] != "a" || ids[2] != "c" {
		t.Fatalf("List = %v, err %v", ids, err)
	}
	if err := store.Apply(ctx, core.ProcessSnapshotChange{DeleteRoots: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, "a"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load deleted error = %v", err)
	}
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
	if decoded["status"] != "running" {
		t.Fatalf("status wire = %#v", decoded["status"])
	}
	for _, version := range []float64{0, 1, 2, 3, 5} {
		decoded["schema_version"] = version
		invalid, _ := json.Marshal(decoded)
		var target core.ProcessSnapshot
		if err := json.Unmarshal(invalid, &target); !errors.Is(err, core.ErrSnapshotSchema) {
			t.Fatalf("schema %v error = %v", version, err)
		}
	}
}

func TestActionRunSnapshotKeepsTypedStatusOnStringWire(t *testing.T) {
	run := core.ActionRunSnapshot{
		ActionName: "lookup",
		StartedAt:  time.Now(),
		Duration:   time.Second,
		Status:     core.ActionSucceeded,
	}
	body, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["status"] != "succeeded" {
		t.Fatalf("status wire = %#v", wire["status"])
	}
	var decoded core.ActionRunSnapshot
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != core.ActionSucceeded {
		t.Fatalf("decoded status = %v", decoded.Status)
	}
	if err := json.Unmarshal([]byte(`{"action":"lookup","started_at":"2026-07-16T08:00:00Z","duration_ns":1,"status":"invented"}`), &decoded); err == nil {
		t.Fatal("unknown action status was accepted")
	}
}

func TestProcessSnapshotRejectsInvalidAggregate(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	invalid := validSnapshot("waiting")
	invalid.Status = core.StatusWaiting
	if err := store.Apply(t.Context(), validSnapshotChange(invalid)); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("waiting without suspension error = %v", err)
	}
	if _, err := store.Load(t.Context(), "missing"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	rootWithDepth := validSnapshot("root-with-depth")
	rootWithDepth.Depth = 1
	if err := store.Apply(t.Context(), validSnapshotChange(rootWithDepth)); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("root with depth error = %v", err)
	}
	childWithoutDepth := validSnapshot("child-without-depth")
	childWithoutDepth.ParentID = "parent"
	if err := store.Apply(t.Context(), validSnapshotChange(childWithoutDepth)); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("child without depth error = %v", err)
	}
	invalidModelCall := validSnapshot("invalid-model-call")
	invalidModelCall.OwnModelCalls = []core.ModelCall{{
		Timestamp: time.Now(), PromptTokens: 1, CompletionTokens: 1, ReasoningTokens: 2,
	}}
	if err := store.Apply(t.Context(), validSnapshotChange(invalidModelCall)); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("invalid model call error = %v", err)
	}
	failedWithoutCause := validSnapshot("failed-without-cause")
	failedWithoutCause.Status = core.StatusFailed
	if err := store.Apply(t.Context(), validSnapshotChange(failedWithoutCause)); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("failed without cause error = %v", err)
	}
	waitingWithFailure := validSnapshot("waiting-with-failure")
	waitingWithFailure.Status = core.StatusWaiting
	waitingWithFailure.Suspension = &interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            "approval", Kind: interaction.SuspensionHuman,
		Prompt: json.RawMessage(`"approve?"`), ResumeSchema: json.RawMessage(`{"type":"boolean"}`), CreatedAt: time.Now(),
	}
	waitingWithFailure.Failure = "must not survive"
	if err := store.Apply(t.Context(), validSnapshotChange(waitingWithFailure)); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("waiting with failure error = %v", err)
	}
}

func TestProcessSnapshotChangeValidatesTreeBoundary(t *testing.T) {
	root := validSnapshot("root")
	child := validSnapshot("child")
	child.ParentID = root.ID
	child.Depth = root.Depth + 1
	disconnected := child
	disconnected.ParentID = "outside"
	wrongDepth := child
	wrongDepth.Depth++

	tests := []struct {
		name   string
		change core.ProcessSnapshotChange
	}{
		{name: "empty change"},
		{name: "empty tree", change: core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{RootID: root.ID}}},
		{name: "missing root", change: core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{child},
		}}},
		{name: "duplicate snapshot", change: core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{root, root},
		}}},
		{name: "external parent", change: core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{root, disconnected},
		}}},
		{name: "wrong depth", change: core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
			RootID: root.ID, Snapshots: []core.ProcessSnapshot{root, wrongDepth},
		}}},
		{name: "save delete conflict", change: core.ProcessSnapshotChange{
			Tree:        &core.ProcessSnapshotTree{RootID: root.ID, Snapshots: []core.ProcessSnapshot{root}},
			DeleteRoots: []string{root.ID},
		}},
		{name: "duplicate delete", change: core.ProcessSnapshotChange{DeleteRoots: []string{"old", "old"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.change.Validate(); !errors.Is(err, core.ErrInvalidSnapshot) {
				t.Fatalf("Validate error = %v, want ErrInvalidSnapshot", err)
			}
		})
	}

	valid := core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
		RootID: root.ID, Snapshots: []core.ProcessSnapshot{child, root},
	}, DeleteRoots: []string{"stale"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid change: %v", err)
	}
}
