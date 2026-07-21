package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// TestProcessStore exercises schema validation, defensive loads, atomic
// mutations, CAS under concurrent writers, and list/delete behavior.
func TestProcessStore(ctx context.Context, store core.ProcessStore) error {
	if store == nil {
		return errors.New("storetest: nil ProcessStore")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("storetest: random ID: %w", err)
	}
	id := "storetest-" + hex.EncodeToString(random[:])
	defer func() {
		_ = store.Apply(context.WithoutCancel(ctx), core.SnapshotMutation{DeleteTrees: []string{id}})
	}()
	started := time.Now().UTC().Add(-time.Second)
	snapshot := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "storetest", Digest: "storetest-digest"},
		StartedAt:     started,
		CapturedAt:    started.Add(time.Millisecond),
		Status:        core.StatusRunning,
		Conditions:    map[string]bool{"durable": true},
	}
	wrongSchema := snapshot
	wrongSchema.SchemaVersion++
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{wrongSchema}}); !errors.Is(err, core.ErrSnapshotSchema) {
		return fmt.Errorf("storetest: unsupported schema error = %w", err)
	}
	invalidWire := snapshot
	invalidWire.Blackboard = map[string]core.TaggedValue{
		"invalid": {Type: "string", Value: []byte("{")},
	}
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{invalidWire}}); !errors.Is(err, core.ErrInvalidSnapshot) {
		return fmt.Errorf("storetest: invalid serialized state error = %w", err)
	}
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{snapshot}}); err != nil {
		return fmt.Errorf("storetest: create snapshot: %w", err)
	}
	loaded, err := store.Load(ctx, id)
	if err != nil || loaded.Revision != 1 || !loaded.Conditions["durable"] {
		return fmt.Errorf("storetest: load revision 1: %#v: %w", loaded, err)
	}
	loaded.Conditions["durable"] = false
	again, err := store.Load(ctx, id)
	if err != nil || !again.Conditions["durable"] {
		return fmt.Errorf("storetest: Load returned mutable stored state: %w", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for tokens := 1; tokens <= 2; tokens++ {
		candidate := again
		candidate.OwnTokens = tokens
		wait.Go(func() {
			<-start
			results <- store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{candidate}})
		})
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, core.ErrRevisionConflict):
			conflicts++
		default:
			return fmt.Errorf("storetest: concurrent CAS: %w", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		return fmt.Errorf("storetest: concurrent CAS successes=%d conflicts=%d", successes, conflicts)
	}
	latest, err := store.Load(ctx, id)
	if err != nil || latest.Revision != 2 {
		return fmt.Errorf("storetest: latest revision = %d: %w", latest.Revision, err)
	}
	if err := testSnapshotMutation(ctx, store, id); err != nil {
		return err
	}

	ids, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("storetest: List: %w", err)
	}
	found := false
	for _, candidate := range ids {
		found = found || candidate == id
	}
	if !found {
		return errors.New("storetest: List omitted committed process")
	}
	if err := store.Apply(ctx, core.SnapshotMutation{DeleteTrees: []string{id}}); err != nil {
		return fmt.Errorf("storetest: Delete: %w", err)
	}
	if err := store.Apply(ctx, core.SnapshotMutation{DeleteTrees: []string{id}}); err != nil {
		return fmt.Errorf("storetest: idempotent Delete: %w", err)
	}
	if _, err := store.Load(ctx, id); !errors.Is(err, core.ErrSnapshotNotFound) {
		return fmt.Errorf("storetest: Load after Delete: %w", err)
	}
	return nil
}

func testSnapshotMutation(
	ctx context.Context,
	store core.ProcessStore,
	prefix string,
) error {
	firstID := prefix + "-batch-first"
	secondID := prefix + "-batch-second"
	defer func() {
		_ = store.Apply(context.WithoutCancel(ctx), core.SnapshotMutation{DeleteTrees: []string{firstID, secondID}})
	}()
	started := time.Now().UTC().Add(-time.Second)
	newSnapshot := func(id string, tokens int) core.ProcessSnapshot {
		return core.ProcessSnapshot{
			SchemaVersion: core.ProcessSnapshotSchemaVersion,
			ID:            id,
			Deployment:    core.DeploymentRef{Name: "storetest", Digest: "storetest-digest"},
			StartedAt:     started,
			CapturedAt:    started.Add(time.Millisecond),
			Status:        core.StatusRunning,
			OwnTokens:     tokens,
		}
	}
	first := newSnapshot(firstID, 1)
	second := newSnapshot(secondID, 2)
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{first, second}}); err != nil {
		return fmt.Errorf("storetest: mutation create: %w", err)
	}

	first.Revision = 1
	first.OwnTokens = 10
	second.OwnTokens = 20 // Deliberately stale: its durable revision is 1.
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{first, second}}); !errors.Is(err, core.ErrRevisionConflict) {
		return fmt.Errorf("storetest: stale mutation error = %w", err)
	}
	storedFirst, err := store.Load(ctx, firstID)
	if err != nil {
		return fmt.Errorf("storetest: load first batch snapshot: %w", err)
	}
	storedSecond, err := store.Load(ctx, secondID)
	if err != nil {
		return fmt.Errorf("storetest: load second batch snapshot: %w", err)
	}
	if storedFirst.Revision != 1 || storedFirst.OwnTokens != 1 || storedSecond.Revision != 1 || storedSecond.OwnTokens != 2 {
		return fmt.Errorf("storetest: rejected mutation changed snapshots: first=%#v second=%#v", storedFirst, storedSecond)
	}

	duplicate := newSnapshot(prefix+"-batch-duplicate", 3)
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{duplicate, duplicate}}); !errors.Is(err, core.ErrInvalidSnapshot) {
		return fmt.Errorf("storetest: duplicate mutation error = %w", err)
	}
	if _, err := store.Load(ctx, duplicate.ID); !errors.Is(err, core.ErrSnapshotNotFound) {
		return fmt.Errorf("storetest: duplicate mutation changed store: %w", err)
	}
	first.Revision = 1
	first.OwnTokens = 100
	if err := store.Apply(ctx, core.SnapshotMutation{
		Writes:      []core.ProcessSnapshot{first},
		DeleteTrees: []string{secondID},
	}); err != nil {
		return fmt.Errorf("storetest: mixed mutation: %w", err)
	}
	storedFirst, err = store.Load(ctx, firstID)
	if err != nil || storedFirst.Revision != 2 || storedFirst.OwnTokens != 100 {
		return fmt.Errorf("storetest: mixed mutation write = %#v: %w", storedFirst, err)
	}
	if _, err := store.Load(ctx, secondID); !errors.Is(err, core.ErrSnapshotNotFound) {
		return fmt.Errorf("storetest: mixed mutation delete: %w", err)
	}
	child := newSnapshot(prefix+"-batch-child", 1)
	child.ParentID = firstID
	child.Depth = 1
	if err := store.Apply(ctx, core.SnapshotMutation{Writes: []core.ProcessSnapshot{child}}); err != nil {
		return fmt.Errorf("storetest: create mutation child: %w", err)
	}
	child.Revision = 1
	child.OwnTokens = 2
	if err := store.Apply(ctx, core.SnapshotMutation{
		Writes:      []core.ProcessSnapshot{child},
		DeleteTrees: []string{firstID},
	}); !errors.Is(err, core.ErrInvalidSnapshot) {
		return fmt.Errorf("storetest: write inside deleted tree error = %w", err)
	}
	if storedChild, err := store.Load(ctx, child.ID); err != nil || storedChild.Revision != 1 || storedChild.OwnTokens != 1 {
		return fmt.Errorf("storetest: rejected tree mutation changed child: %#v: %w", storedChild, err)
	}
	return nil
}
