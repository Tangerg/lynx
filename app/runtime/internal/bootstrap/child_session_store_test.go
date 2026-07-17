package bootstrap

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/storetest"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestChildSessionStorePreservesRuntimeIdentity(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	productSessions := sqlite.NewSessionStore(db)
	parent, err := productSessions.Create(t.Context(), "parent", "/workspace")
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	store := newChildSessionStore(productSessions)
	now := time.Unix(1_700_000_000, 123).UTC()
	session := core.Session{
		ID:        "child-process",
		ParentID:  parent.ID,
		UserID:    "user-1",
		AgentName: "research-agent",
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		Metadata:  map[string]any{"source": "runtime"},
	}

	if err := store.Save(t.Context(), session); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != session.ID || loaded.ParentID != session.ParentID ||
		loaded.UserID != session.UserID || loaded.AgentName != session.AgentName ||
		!loaded.StartedAt.Equal(session.StartedAt) || !loaded.UpdatedAt.Equal(session.UpdatedAt) ||
		loaded.Metadata["source"] != "runtime" {
		t.Fatalf("loaded session = %#v, want runtime identity %#v", loaded, session)
	}
	product, err := productSessions.Get(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("Get product session: %v", err)
	}
	if product.Kind != sessionsvc.KindSubtask || product.Cwd != "/workspace" {
		t.Fatalf("product enrichment = %#v", product)
	}
}

func TestChildSessionStoreMapsNotFound(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newChildSessionStore(sqlite.NewSessionStore(db))

	if _, err := store.Load(t.Context(), "missing"); !errors.Is(err, core.ErrSessionNotFound) {
		t.Fatalf("Load error = %v, want ErrSessionNotFound", err)
	}
}

func TestChildSessionStoreRejectsUserFacingSession(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	productSessions := sqlite.NewSessionStore(db)
	root, err := productSessions.Create(t.Context(), "root", "/workspace")
	if err != nil {
		t.Fatalf("Create root: %v", err)
	}
	store := newChildSessionStore(productSessions)

	if _, err := store.Load(t.Context(), root.ID); !errors.Is(err, sessionsvc.ErrSubtaskConflict) {
		t.Fatalf("Load root error = %v, want ErrSubtaskConflict", err)
	}
}

func TestChildSessionStoreContract(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newChildSessionStore(sqlite.NewSessionStore(db))

	if err := storetest.TestSessionStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
}
