package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func TestSessionPatchEmptyDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	store := newTempDB(t)
	created, err := store.Create(ctx, "unchanged", "/tmp")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Patch(ctx, created.ID, session.Patch{ExpectedRevision: created.Revision})
	if err != nil {
		t.Fatalf("Patch empty: %v", err)
	}
	if got.Revision != created.Revision {
		t.Fatalf("Revision = %d, want unchanged %d", got.Revision, created.Revision)
	}
	if !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("UpdatedAt = %v, want unchanged %v", got.UpdatedAt, created.UpdatedAt)
	}
}

func TestSessionPatchEmptyStillChecksRevision(t *testing.T) {
	ctx := context.Background()
	store := newTempDB(t)
	created, err := store.Create(ctx, "unchanged", "/tmp")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = store.Patch(ctx, created.ID, session.Patch{ExpectedRevision: created.Revision + 1})
	if !errors.Is(err, session.ErrRevisionConflict) {
		t.Fatalf("Patch empty stale revision = %v, want ErrRevisionConflict", err)
	}
}
