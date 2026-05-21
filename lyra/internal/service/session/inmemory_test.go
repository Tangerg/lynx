package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

func TestInMemoryService_CreateGetList(t *testing.T) {
	svc := session.NewInMemoryService()
	ctx := context.Background()

	created, err := svc.Create(ctx, "first")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created session has empty ID")
	}
	if created.Title != "first" {
		t.Errorf("Title = %q, want %q", created.Title, "first")
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get returned %q, want %q", got.ID, created.ID)
	}

	all, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List len = %d, want 1", len(all))
	}
}

func TestInMemoryService_GetUnknown(t *testing.T) {
	svc := session.NewInMemoryService()
	_, err := svc.Get(context.Background(), "nope")
	if !errors.Is(err, session.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestInMemoryService_Fork(t *testing.T) {
	svc := session.NewInMemoryService()
	ctx := context.Background()

	parent, _ := svc.Create(ctx, "main")
	child, err := svc.Fork(ctx, parent.ID, "msg-7")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Errorf("ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Metadata["fork_at_message_id"] != "msg-7" {
		t.Errorf("metadata = %v", child.Metadata)
	}
}

func TestInMemoryService_ForkUnknownParent(t *testing.T) {
	svc := session.NewInMemoryService()
	_, err := svc.Fork(context.Background(), "no-such", "any")
	if !errors.Is(err, session.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestInMemoryService_DeleteIdempotent(t *testing.T) {
	svc := session.NewInMemoryService()
	ctx := context.Background()

	sess, _ := svc.Create(ctx, "to-delete")
	if err := svc.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.Delete(ctx, sess.ID); err != nil {
		t.Errorf("Delete idempotent: %v", err)
	}
	if err := svc.Delete(ctx, "never-existed"); err != nil {
		t.Errorf("Delete unknown: %v", err)
	}
	if _, err := svc.Get(ctx, sess.ID); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("after Delete, Get err = %v, want ErrNotFound", err)
	}
}
