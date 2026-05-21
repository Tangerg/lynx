package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestNewSession_DefaultsTimestamps(t *testing.T) {
	before := time.Now()
	sess := core.NewSession("s-1", "user-1", "demo-agent")
	after := time.Now()

	if sess.ID != "s-1" || sess.UserID != "user-1" || sess.AgentName != "demo-agent" {
		t.Errorf("identity: %#v", sess)
	}
	if sess.StartedAt.Before(before) || sess.StartedAt.After(after) {
		t.Errorf("StartedAt: outside [%v, %v]: %v", before, after, sess.StartedAt)
	}
	if !sess.StartedAt.Equal(sess.UpdatedAt) {
		t.Errorf("StartedAt should equal UpdatedAt at creation; got %v vs %v",
			sess.StartedAt, sess.UpdatedAt)
	}
	if sess.Metadata == nil {
		t.Error("Metadata should be allocated, not nil")
	}
}

func TestSession_Touch_RefreshesUpdatedAt(t *testing.T) {
	sess := core.NewSession("s-1", "u", "demo")
	before := sess.UpdatedAt
	time.Sleep(2 * time.Millisecond)
	sess.Touch()
	if !sess.UpdatedAt.After(before) {
		t.Errorf("Touch should advance UpdatedAt: before=%v after=%v", before, sess.UpdatedAt)
	}
}

func TestSession_Touch_NilSafe(t *testing.T) {
	// Must not panic.
	var sess *core.Session
	sess.Touch()
}

func TestInMemorySessionStore_SaveLoad(t *testing.T) {
	store := core.NewInMemorySessionStore()
	ctx := context.Background()

	sess := core.NewSession("s-1", "user-1", "demo")
	sess.Metadata["channel"] = "web"

	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(ctx, "s-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != "s-1" || got.Metadata["channel"] != "web" {
		t.Errorf("round-trip: %#v", got)
	}
}

func TestInMemorySessionStore_NotFound(t *testing.T) {
	store := core.NewInMemorySessionStore()
	_, err := store.Load(context.Background(), "ghost")
	if !errors.Is(err, core.ErrSessionNotFound) {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

func TestInMemorySessionStore_DeleteIdempotent(t *testing.T) {
	store := core.NewInMemorySessionStore()
	ctx := context.Background()

	_ = store.Save(ctx, core.NewSession("s-1", "u", "x"))
	if err := store.Delete(ctx, "s-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "s-1"); err != nil {
		t.Errorf("redelete: %v", err)
	}
	if err := store.Delete(ctx, "never-existed"); err != nil {
		t.Errorf("delete unknown: %v", err)
	}
}

func TestInMemorySessionStore_SaveEmptyID(t *testing.T) {
	if err := core.NewInMemorySessionStore().Save(context.Background(), core.Session{}); err == nil {
		t.Error("expected error for empty session ID")
	}
}
