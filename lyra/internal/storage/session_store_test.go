package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

// withTempHome points LYRA_HOME at a per-test temp dir for the
// duration of the test. Every storage constructor opens
// <LYRA_HOME>/... so this gives full isolation without mocking.
func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("LYRA_HOME", t.TempDir())
}

func TestFileSessionService_CreateGetList(t *testing.T) {
	withTempHome(t)

	svc, err := storage.NewFileSessionService()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()

	a, _ := svc.Create(ctx, "first", "")
	b, _ := svc.Create(ctx, "second", "")

	list, _ := svc.List(ctx)
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	gotA, _ := svc.Get(ctx, a.ID)
	if gotA.Title != "first" {
		t.Errorf("Get returned title = %q", gotA.Title)
	}
	if b.ID == a.ID {
		t.Error("Create produced colliding IDs")
	}
}

// TestFileSessionService_PersistsAcrossInstances proves the
// persistence promise — a second NewFileSessionService over the
// same LYRA_HOME sees what the first one wrote.
func TestFileSessionService_PersistsAcrossInstances(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()

	first, err := storage.NewFileSessionService()
	if err != nil {
		t.Fatal(err)
	}
	created, _ := first.Create(ctx, "remember me", "")

	// Simulate a fresh process restart.
	second, err := storage.NewFileSessionService()
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if got.Title != "remember me" {
		t.Errorf("Title after restart = %q", got.Title)
	}
}

func TestFileSessionService_ForkRecordsParent(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()
	svc, _ := storage.NewFileSessionService()

	parent, _ := svc.Create(ctx, "main", "")
	child, err := svc.Fork(ctx, parent.ID, "msg-3")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Errorf("ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Metadata["fork_at_message_id"] != "msg-3" {
		t.Errorf("fork metadata = %v", child.Metadata)
	}
}

func TestFileSessionService_DeletePersists(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()

	svc, _ := storage.NewFileSessionService()
	sess, _ := svc.Create(ctx, "to-delete", "")
	if err := svc.Delete(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}

	reopened, _ := storage.NewFileSessionService()
	if _, err := reopened.Get(ctx, sess.ID); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Get after restart of deleted session: err = %v, want ErrNotFound", err)
	}
}

func TestFileSessionService_TouchBumpsTurnCount(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()

	svc, _ := storage.NewFileSessionService()
	sess, _ := svc.Create(ctx, "", "")
	for i := 0; i < 3; i++ {
		if err := svc.Touch(sess.ID); err != nil {
			t.Fatalf("Touch %d: %v", i, err)
		}
	}

	got, _ := svc.Get(ctx, sess.ID)
	if got.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", got.TurnCount)
	}
}
