package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestNewSessionDefaultsTimestamps(t *testing.T) {
	before := time.Now()
	session := core.NewSession("s-1", "user-1", "demo-agent")
	after := time.Now()

	if session.ID != "s-1" || session.UserID != "user-1" || session.AgentName != "demo-agent" {
		t.Errorf("identity: %#v", session)
	}
	if session.StartedAt.Before(before) || session.StartedAt.After(after) {
		t.Errorf("StartedAt: outside [%v, %v]: %v", before, after, session.StartedAt)
	}
	if !session.StartedAt.Equal(session.UpdatedAt) {
		t.Errorf("StartedAt should equal UpdatedAt at creation; got %v vs %v",
			session.StartedAt, session.UpdatedAt)
	}
	if session.Metadata == nil {
		t.Error("Metadata should be allocated, not nil")
	}
}

func TestSessionTouchRefreshesUpdatedAt(t *testing.T) {
	session := core.NewSession("s-1", "u", "demo")
	before := time.Unix(1, 0).UTC()
	session.UpdatedAt = before
	session.Touch()
	if !session.UpdatedAt.After(before) {
		t.Errorf("Touch should advance UpdatedAt: before=%v after=%v", before, session.UpdatedAt)
	}
}

func TestMemorySessionStoreSaveLoad(t *testing.T) {
	store := core.NewMemorySessionStore()
	ctx := context.Background()

	session := core.NewSession("s-1", "user-1", "demo")
	session.Metadata["channel"] = "web"

	if err := store.Save(ctx, session); err != nil {
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

func TestMemorySessionStoreNotFound(t *testing.T) {
	store := core.NewMemorySessionStore()
	_, err := store.Load(context.Background(), "ghost")
	if !errors.Is(err, core.ErrSessionNotFound) {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

func TestMemorySessionStoreDeleteIdempotent(t *testing.T) {
	store := core.NewMemorySessionStore()
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

func TestMemorySessionStoreRejectsEmptyID(t *testing.T) {
	if err := core.NewMemorySessionStore().Save(context.Background(), core.Session{}); err == nil {
		t.Error("expected error for empty session ID")
	}
}
