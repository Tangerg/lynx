package runtimeembedded

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type sessionCatalogStub struct {
	pages map[string]*protocol.Page[protocol.Session]
}

func (stub sessionCatalogStub) ListSessions(_ context.Context, query protocol.PageQuery, _ embedded.CallOptions) (*protocol.Page[protocol.Session], error) {
	return stub.pages[query.Cursor], nil
}

func (sessionCatalogStub) CreateSession(context.Context, protocol.CreateSessionRequest, embedded.CommandOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected CreateSession")
}

func (sessionCatalogStub) UpdateSession(context.Context, protocol.UpdateSessionRequest, embedded.CommandOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected UpdateSession")
}

func (sessionCatalogStub) ForkSession(context.Context, protocol.ForkSessionRequest, embedded.CommandOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected ForkSession")
}

func (sessionCatalogStub) DeleteSession(context.Context, protocol.DeleteSessionRequest, embedded.CommandOptions) error {
	return errors.New("unexpected DeleteSession")
}

func TestFilteredSessionCatalogRejectsMultiStepCursorCycle(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{sessionCatalog: sessionCatalogStub{pages: map[string]*protocol.Page[protocol.Session]{
		"":       protocol.NewPageWithCursor([]protocol.Session{}, "first"),
		"first":  protocol.NewPageWithCursor([]protocol.Session{}, "second"),
		"second": protocol.NewPageWithCursor([]protocol.Session{}, "first"),
	}}, meta: requestMeta("test")}

	_, err := runtime.ListSessions(t.Context(), agent.SessionQuery{Search: "needle"})
	if err == nil || !strings.Contains(err.Error(), "cyclic continuation cursor") {
		t.Fatalf("ListSessions error = %v, want cursor cycle failure", err)
	}
}
