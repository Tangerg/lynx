package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// TestProcessStore exercises snapshot validation, replacement, ownership-
// isolated loads, listing, and process-tree deletion. Storage coordination and
// transaction behavior belong to each implementation and are deliberately
// outside this contract suite.
func TestProcessStore(ctx context.Context, store core.ProcessStore) error {
	if store == nil {
		return errors.New("storetest: nil ProcessStore")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("storetest: random ID: %w", err)
	}
	prefix := "storetest-" + hex.EncodeToString(random[:])
	firstID := prefix + "-first"
	secondID := prefix + "-second"
	defer func() {
		_ = store.Delete(context.WithoutCancel(ctx), firstID)
		_ = store.Delete(context.WithoutCancel(ctx), secondID)
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
			Conditions:    map[string]bool{"durable": true},
		}
	}

	first := newSnapshot(firstID, 1)
	wrongSchema := first
	wrongSchema.SchemaVersion++
	if err := store.Save(ctx, []core.ProcessSnapshot{wrongSchema}); !errors.Is(err, core.ErrSnapshotSchema) {
		return fmt.Errorf("storetest: unsupported schema error = %w", err)
	}
	invalid := first
	invalid.Blackboard = map[string]core.TaggedValue{
		"invalid": {Type: "string", Value: []byte("{")},
	}
	if err := store.Save(ctx, []core.ProcessSnapshot{invalid}); !errors.Is(err, core.ErrInvalidSnapshot) {
		return fmt.Errorf("storetest: invalid serialized state error = %w", err)
	}

	second := newSnapshot(secondID, 2)
	if err := store.Save(ctx, []core.ProcessSnapshot{first, second}); err != nil {
		return fmt.Errorf("storetest: save snapshots: %w", err)
	}
	loaded, err := store.Load(ctx, firstID)
	if err != nil || loaded.OwnTokens != 1 || !loaded.Conditions["durable"] {
		return fmt.Errorf("storetest: load snapshot: %#v: %w", loaded, err)
	}
	loaded.Conditions["durable"] = false
	again, err := store.Load(ctx, firstID)
	if err != nil || !again.Conditions["durable"] {
		return fmt.Errorf("storetest: Load returned mutable stored state: %w", err)
	}

	first.OwnTokens = 3
	if err := store.Save(ctx, []core.ProcessSnapshot{first}); err != nil {
		return fmt.Errorf("storetest: replace snapshot: %w", err)
	}
	replaced, err := store.Load(ctx, firstID)
	if err != nil || replaced.OwnTokens != 3 {
		return fmt.Errorf("storetest: replaced snapshot = %#v: %w", replaced, err)
	}

	if lister, ok := store.(core.ProcessLister); ok {
		ids, err := lister.List(ctx)
		if err != nil {
			return fmt.Errorf("storetest: List: %w", err)
		}
		found := false
		for _, candidate := range ids {
			found = found || candidate == firstID
		}
		if !found {
			return errors.New("storetest: List omitted saved process")
		}
	}
	if err := store.Delete(ctx, firstID); err != nil {
		return fmt.Errorf("storetest: Delete first process tree: %w", err)
	}
	if err := store.Delete(ctx, secondID); err != nil {
		return fmt.Errorf("storetest: Delete: %w", err)
	}
	if _, err := store.Load(ctx, firstID); !errors.Is(err, core.ErrSnapshotNotFound) {
		return fmt.Errorf("storetest: Load after Delete: %w", err)
	}
	return nil
}
