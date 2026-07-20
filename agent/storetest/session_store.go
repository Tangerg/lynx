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

// TestSessionStore exercises validation, replacement, ownership isolation,
// concurrent writes, not-found classification, and optional administrative
// capabilities. It uses unique IDs and cleans up when the store implements
// [core.SessionDeleter].
func TestSessionStore(ctx context.Context, store core.SessionStore) error {
	if store == nil {
		return errors.New("storetest: nil SessionStore")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("storetest: random ID: %w", err)
	}
	id := "storetest-session-" + hex.EncodeToString(random[:])
	if deleter, ok := store.(core.SessionDeleter); ok {
		defer func() { _ = deleter.Delete(context.WithoutCancel(ctx), id) }()
	}

	invalid := core.NewSession("", "user-1", "storetest")
	if err := store.Save(ctx, invalid); !errors.Is(err, core.ErrInvalidSession) {
		return fmt.Errorf("storetest: invalid session error = %w", err)
	}
	session := core.NewSession(id, "user-1", "storetest")
	session.ParentID = id + "-parent"
	labels := []any{"saved"}
	if err := session.Metadata.Set("labels", labels); err != nil {
		return fmt.Errorf("storetest: set labels: %w", err)
	}
	if err := store.Save(ctx, session); err != nil {
		return fmt.Errorf("storetest: create session: %w", err)
	}
	labels[0] = "caller-mutated"

	loaded, err := store.Load(ctx, id)
	if err != nil {
		return fmt.Errorf("storetest: load session: %w", err)
	}
	loadedLabels, err := metadataValue[[]any](loaded.Metadata, "labels")
	if err != nil {
		return fmt.Errorf("storetest: decode saved labels: %w", err)
	}
	if got := loadedLabels[0]; got != "saved" {
		return fmt.Errorf("storetest: Save retained caller metadata: %v", got)
	}
	loadedLabels[0] = "load-mutated"
	again, err := store.Load(ctx, id)
	if err != nil {
		return fmt.Errorf("storetest: reload session: %w", err)
	}
	againLabels, err := metadataValue[[]any](again.Metadata, "labels")
	if err != nil {
		return fmt.Errorf("storetest: decode reloaded labels: %w", err)
	}
	if got := againLabels[0]; got != "saved" {
		return fmt.Errorf("storetest: Load returned mutable stored metadata: %v", got)
	}

	again.UpdatedAt = again.UpdatedAt.Add(time.Second)
	again.Metadata = core.SessionMetadata{}
	if err := again.Metadata.Set("replacement", true); err != nil {
		return fmt.Errorf("storetest: set replacement metadata: %w", err)
	}
	if err := store.Save(ctx, again); err != nil {
		return fmt.Errorf("storetest: replace session: %w", err)
	}
	replaced, err := store.Load(ctx, id)
	if err != nil {
		return fmt.Errorf("storetest: load replaced session: %w", err)
	}
	replacement, err := metadataValue[bool](replaced.Metadata, "replacement")
	if err != nil {
		return fmt.Errorf("storetest: decode replacement metadata: %w", err)
	}
	if !replaced.UpdatedAt.Equal(again.UpdatedAt) || !replacement {
		return fmt.Errorf("storetest: replaced session = %#v, want updated timestamp and replacement marker", replaced)
	}

	start := make(chan struct{})
	results := make(chan error, 4)
	var wait sync.WaitGroup
	for index := range 4 {
		candidate := replaced
		candidate.Metadata = core.SessionMetadata{}
		if err := candidate.Metadata.Set("writer", index); err != nil {
			return fmt.Errorf("storetest: set writer metadata: %w", err)
		}
		wait.Go(func() {
			<-start
			results <- store.Save(ctx, candidate)
		})
	}
	close(start)
	wait.Wait()
	close(results)
	for result := range results {
		if result != nil {
			return fmt.Errorf("storetest: concurrent Save: %w", result)
		}
	}
	if _, err := store.Load(ctx, id+"-missing"); !errors.Is(err, core.ErrSessionNotFound) {
		return fmt.Errorf("storetest: missing session error = %w", err)
	}

	if lister, ok := store.(core.SessionLister); ok {
		ids, err := lister.List(ctx)
		if err != nil {
			return fmt.Errorf("storetest: List: %w", err)
		}
		found := false
		for _, candidate := range ids {
			found = found || candidate == id
		}
		if !found {
			return errors.New("storetest: List omitted committed session")
		}
	}
	if deleter, ok := store.(core.SessionDeleter); ok {
		if err := deleter.Delete(ctx, id); err != nil {
			return fmt.Errorf("storetest: Delete: %w", err)
		}
		if err := deleter.Delete(ctx, id); err != nil {
			return fmt.Errorf("storetest: idempotent Delete: %w", err)
		}
		if _, err := store.Load(ctx, id); !errors.Is(err, core.ErrSessionNotFound) {
			return fmt.Errorf("storetest: Load after Delete: %w", err)
		}
	}
	return nil
}

func metadataValue[T any](metadata core.SessionMetadata, name string) (T, error) {
	var value T
	found, err := metadata.Decode(name, &value)
	if err != nil {
		return value, err
	}
	if !found {
		return value, fmt.Errorf("metadata field %q is missing", name)
	}
	return value, nil
}
