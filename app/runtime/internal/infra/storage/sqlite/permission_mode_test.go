package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newPermissionModeStores(t *testing.T) (*sqlite.PermissionModeStore, *sqlite.SessionStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewPermissionModeStore(db), sqlite.NewSessionStore(db)
}

func TestPermissionModeStoreRoundTripAndSessionLifecycle(t *testing.T) {
	modes, sessions := newPermissionModeStores(t)
	created, err := sessions.Create(t.Context(), "Plan session", "/repo")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, found, err := modes.GetMode(t.Context(), created.ID); err != nil || found {
		t.Fatalf("GetMode before entry = found %v, err %v", found, err)
	}
	want := approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeBalanced}
	if err := modes.PutMode(t.Context(), created.ID, want); err != nil {
		t.Fatalf("PutMode: %v", err)
	}
	got, found, err := modes.GetMode(t.Context(), created.ID)
	if err != nil || !found || got != want {
		t.Fatalf("GetMode = %+v, found %v, err %v; want %+v", got, found, err, want)
	}

	restored := approval.SessionMode{Mode: approval.ModeBalanced}
	if err := modes.PutMode(t.Context(), created.ID, restored); err != nil {
		t.Fatalf("PutMode(restored): %v", err)
	}
	if got, found, err = modes.GetMode(t.Context(), created.ID); err != nil || !found || got != restored {
		t.Fatalf("GetMode(restored) = %+v, found %v, err %v", got, found, err)
	}

	if err := sessions.Delete(t.Context(), created.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, found, err = modes.GetMode(t.Context(), created.ID); err != nil || found {
		t.Fatalf("GetMode after session delete = found %v, err %v", found, err)
	}
}

func TestPermissionModeStoreRejectsInvalidState(t *testing.T) {
	modes, sessions := newPermissionModeStores(t)
	created, err := sessions.Create(t.Context(), "Plan session", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	invalid := approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModePlan}
	if err := modes.PutMode(t.Context(), created.ID, invalid); err == nil {
		t.Fatal("PutMode accepted a Plan-to-Plan restore cycle")
	}
	if _, found, err := modes.GetMode(t.Context(), created.ID); err != nil || found {
		t.Fatalf("invalid write left a row: found %v, err %v", found, err)
	}
}
