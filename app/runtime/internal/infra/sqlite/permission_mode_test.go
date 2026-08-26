package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

func newPermissionModeStores(t *testing.T) (*sqlite.PermissionModeStore, *sqlite.SessionStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewPermissionModeStore(db), sqlite.NewSessionStore(db)
}

func TestPermissionModeStoreRoundTripAndSessionLifecycle(t *testing.T) {
	modes, sessions := newPermissionModeStores(t)
	created := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_plan", Title: "Plan session", Workspace: sessionfixture.MustWorkspace("/repo"),
	})
	if err := sessions.Insert(t.Context(), created); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, found, err := modes.LookupMode(t.Context(), created.ID()); err != nil || found {
		t.Fatalf("LookupMode before entry = found %v, err %v", found, err)
	}
	want := approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeBalanced}
	if err := modes.PutMode(t.Context(), created.ID(), want); err != nil {
		t.Fatalf("PutMode: %v", err)
	}
	got, found, err := modes.LookupMode(t.Context(), created.ID())
	if err != nil || !found || got != want {
		t.Fatalf("LookupMode = %+v, found %v, err %v; want %+v", got, found, err, want)
	}

	restored := approval.SessionMode{Mode: approval.ModeBalanced}
	if putModeErr := modes.PutMode(t.Context(), created.ID(), restored); putModeErr != nil {
		t.Fatalf("PutMode(restored): %v", putModeErr)
	}
	if got, found, err = modes.LookupMode(t.Context(), created.ID()); err != nil || !found || got != restored {
		t.Fatalf("LookupMode(restored) = %+v, found %v, err %v", got, found, err)
	}

	if deleteErr := sessions.Delete(t.Context(), created.ID()); deleteErr != nil {
		t.Fatalf("delete session: %v", deleteErr)
	}
	if _, found, err = modes.LookupMode(t.Context(), created.ID()); err != nil || found {
		t.Fatalf("LookupMode after session delete = found %v, err %v", found, err)
	}
}

func TestPermissionModeStoreRejectsInvalidState(t *testing.T) {
	modes, sessions := newPermissionModeStores(t)
	created := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_invalid_plan", Title: "Plan session", Workspace: sessionfixture.MustWorkspace("/repo"),
	})
	if err := sessions.Insert(t.Context(), created); err != nil {
		t.Fatal(err)
	}
	invalid := approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModePlan}
	if err := modes.PutMode(t.Context(), created.ID(), invalid); err == nil {
		t.Fatal("PutMode accepted a Plan-to-Plan restore cycle")
	}
	if _, found, err := modes.LookupMode(t.Context(), created.ID()); err != nil || found {
		t.Fatalf("invalid write left a row: found %v, err %v", found, err)
	}
}
