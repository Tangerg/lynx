package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
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

func TestMemoryProcessStoreCASAndDefensiveLoad(t *testing.T) {
	store := core.NewMemoryProcessStore()
	ctx := context.Background()
	snapshot := validSnapshot("p-1")
	snapshot.OwnTokens = 1500

	err := store.Save(ctx, snapshot)
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	loaded, err := store.Load(ctx, snapshot.ID)
	if err != nil || loaded.Revision != 1 || loaded.OwnTokens != 1500 {
		t.Fatalf("Load = %#v, err %v", loaded, err)
	}
	loaded.OwnTokens = 99
	again, _ := store.Load(ctx, snapshot.ID)
	if again.OwnTokens != 1500 {
		t.Fatal("Load returned mutable stored state")
	}

	snapshot.Revision = 1
	snapshot.OwnTokens = 2000
	if err := store.Save(ctx, snapshot); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	stale := snapshot
	stale.Revision = 1
	if err := store.Save(ctx, stale); !errors.Is(err, core.ErrRevisionConflict) {
		t.Fatalf("stale Save error = %v", err)
	} else {
		var conflict *core.RevisionConflictError
		if !errors.As(err, &conflict) || conflict.Expected != 1 || conflict.Actual != 2 {
			t.Fatalf("conflict = %#v", conflict)
		}
	}
}

func TestMemoryProcessStoreSaveBatchIsAtomic(t *testing.T) {
	store := core.NewMemoryProcessStore()
	first := validSnapshot("first")
	first.OwnTokens = 1
	second := validSnapshot("second")
	second.OwnTokens = 2

	err := store.SaveBatch(t.Context(), []core.ProcessSnapshot{first, second})
	if err != nil {
		t.Fatalf("first SaveBatch: %v", err)
	}

	first.Revision = 1
	first.OwnTokens = 10
	second.Revision = 0 // stale: durable revision is already 1.
	second.OwnTokens = 20
	err = store.SaveBatch(t.Context(), []core.ProcessSnapshot{first, second})
	if !errors.Is(err, core.ErrRevisionConflict) {
		t.Fatalf("stale SaveBatch error = %v, want revision conflict", err)
	}
	storedFirst, err := store.Load(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedSecond, err := store.Load(t.Context(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedFirst.Revision != 1 || storedFirst.OwnTokens != 1 || storedSecond.Revision != 1 || storedSecond.OwnTokens != 2 {
		t.Fatalf("stored batch after rejected CAS = %#v / %#v, want both original revisions", storedFirst, storedSecond)
	}
}

func TestMemoryProcessStoreSaveBatchRejectsDuplicateIdentity(t *testing.T) {
	store := core.NewMemoryProcessStore()
	snapshot := validSnapshot("duplicate")
	err := store.SaveBatch(t.Context(), []core.ProcessSnapshot{snapshot, snapshot})
	if !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("duplicate SaveBatch error = %v, want invalid snapshot", err)
	}
	if _, err := store.Load(t.Context(), snapshot.ID); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("duplicate SaveBatch mutated store: %v", err)
	}
}

func TestMemoryProcessStoreManagementCapabilities(t *testing.T) {
	store := core.NewMemoryProcessStore()
	ctx := context.Background()
	for _, id := range []string{"c", "a", "b"} {
		if err := store.Save(ctx, validSnapshot(id)); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil || len(ids) != 3 || ids[0] != "a" || ids[2] != "c" {
		t.Fatalf("List = %v, err %v", ids, err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatalf("idempotent Delete: %v", err)
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
	for _, version := range []float64{0, 1, 2, 4} {
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
		Attempts:   1,
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
	if err := json.Unmarshal([]byte(`{"action":"lookup","started_at":"2026-07-16T08:00:00Z","duration_ns":1,"status":"invented","attempts":1}`), &decoded); err == nil {
		t.Fatal("unknown action status was accepted")
	}
}

func TestProcessSnapshotRejectsInvalidAggregate(t *testing.T) {
	store := core.NewMemoryProcessStore()
	invalid := validSnapshot("waiting")
	invalid.Status = core.StatusWaiting
	if err := store.Save(t.Context(), invalid); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("waiting without suspension error = %v", err)
	}
	if _, err := store.Load(t.Context(), "missing"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	invalidModelCall := validSnapshot("invalid-model-call")
	invalidModelCall.OwnModelCalls = []core.ModelCall{{
		Timestamp: time.Now(), PromptTokens: 1, CompletionTokens: 1, ReasoningTokens: 2,
	}}
	if err := store.Save(t.Context(), invalidModelCall); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("invalid model call error = %v", err)
	}
}
