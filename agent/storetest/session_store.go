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
	session.Metadata["labels"] = labels
	if err := store.Save(ctx, session); err != nil {
		return fmt.Errorf("storetest: create session: %w", err)
	}
	labels[0] = "caller-mutated"

	loaded, err := store.Load(ctx, id)
	if err != nil {
		return fmt.Errorf("storetest: load session: %w", err)
	}
	if got := loaded.Metadata["labels"].([]any)[0]; got != "saved" {
		return fmt.Errorf("storetest: Save retained caller metadata: %v", got)
	}
	loaded.Metadata["labels"].([]any)[0] = "load-mutated"
	again, err := store.Load(ctx, id)
	if err != nil {
		return fmt.Errorf("storetest: reload session: %w", err)
	}
	if got := again.Metadata["labels"].([]any)[0]; got != "saved" {
		return fmt.Errorf("storetest: Load returned mutable stored metadata: %v", got)
	}

	again.UpdatedAt = again.UpdatedAt.Add(time.Second)
	again.Metadata = map[string]any{"replacement": true}
	if err := store.Save(ctx, again); err != nil {
		return fmt.Errorf("storetest: replace session: %w", err)
	}
	replaced, err := store.Load(ctx, id)
	if err != nil || !replaced.UpdatedAt.Equal(again.UpdatedAt) || replaced.Metadata["replacement"] != true {
		return fmt.Errorf("storetest: replaced session = %#v: %w", replaced, err)
	}

	start := make(chan struct{})
	results := make(chan error, 4)
	var wait sync.WaitGroup
	for index := range 4 {
		candidate := replaced
		candidate.Metadata = map[string]any{"writer": index}
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
