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

// TestProcessStore exercises schema validation, defensive loads, CAS under
// concurrent writers, and optional list/delete capabilities. It uses a unique
// process ID and cleans it up when the store implements [core.SnapshotDeleter].
func TestProcessStore(ctx context.Context, store core.ProcessStore) error {
	if store == nil {
		return errors.New("storetest: nil ProcessStore")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("storetest: random ID: %w", err)
	}
	id := "storetest-" + hex.EncodeToString(random[:])
	if deleter, ok := store.(core.SnapshotDeleter); ok {
		defer func() { _ = deleter.Delete(context.WithoutCancel(ctx), id) }()
	}
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
	if _, err := store.Save(ctx, wrongSchema, 0); !errors.Is(err, core.ErrSnapshotSchema) {
		return fmt.Errorf("storetest: unsupported schema error = %w", err)
	}
	invalidWire := snapshot
	invalidWire.Blackboard = map[string]core.TaggedValue{
		"invalid": {Type: "string", Value: []byte("{")},
	}
	if _, err := store.Save(ctx, invalidWire, 0); !errors.Is(err, core.ErrInvalidSnapshot) {
		return fmt.Errorf("storetest: invalid serialized state error = %w", err)
	}
	revision, err := store.Save(ctx, snapshot, 0)
	if err != nil || revision != 1 {
		return fmt.Errorf("storetest: create revision = %d: %w", revision, err)
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
			_, saveErr := store.Save(ctx, candidate, 1)
			results <- saveErr
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

	if lister, ok := store.(core.SnapshotLister); ok {
		ids, err := lister.List(ctx)
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
	}
	if deleter, ok := store.(core.SnapshotDeleter); ok {
		if err := deleter.Delete(ctx, id); err != nil {
			return fmt.Errorf("storetest: Delete: %w", err)
		}
		if err := deleter.Delete(ctx, id); err != nil {
			return fmt.Errorf("storetest: idempotent Delete: %w", err)
		}
		if _, err := store.Load(ctx, id); !errors.Is(err, core.ErrSnapshotNotFound) {
			return fmt.Errorf("storetest: Load after Delete: %w", err)
		}
	}
	return nil
}
