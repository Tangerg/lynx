package session

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
)

func TestOpenCreatesOrRestoresAValidatedSnapshot(t *testing.T) {
	runtime := mock.New()

	created, err := Open(t.Context(), runtime, "", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Session.ID == "" || len(created.Events) != 0 || created.Active != nil {
		t.Fatalf("created snapshot = %+v", created)
	}

	restored, err := Open(t.Context(), runtime, created.Session.ID, "ignored")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Session != created.Session {
		t.Fatalf("restored session = %+v, want %+v", restored.Session, created.Session)
	}
}

func TestOpenPreservesRuntimeErrorIdentity(t *testing.T) {
	_, err := Open(t.Context(), mock.New(), "missing", "")
	if !errors.Is(err, client.ErrSessionNotFound) {
		t.Fatalf("open error = %v, want session-not-found identity", err)
	}
}
