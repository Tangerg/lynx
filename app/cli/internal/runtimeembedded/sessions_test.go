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
	pages  map[string]*protocol.Page[protocol.Session]
	update func(protocol.UpdateSessionRequest) (*protocol.Session, error)
}

func (stub sessionCatalogStub) ListSessions(_ context.Context, query protocol.PageQuery, _ embedded.CallOptions) (*protocol.Page[protocol.Session], error) {
	return stub.pages[query.Cursor], nil
}

func (sessionCatalogStub) CreateSession(context.Context, protocol.CreateSessionRequest, embedded.CommandOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected CreateSession")
}

func (stub sessionCatalogStub) UpdateSession(_ context.Context, request protocol.UpdateSessionRequest, _ embedded.CommandOptions) (*protocol.Session, error) {
	if stub.update != nil {
		return stub.update(request)
	}
	return nil, errors.New("unexpected UpdateSession")
}

func (sessionCatalogStub) ForkSession(context.Context, protocol.ForkSessionRequest, embedded.CommandOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected ForkSession")
}

func TestUpdateSessionProjectsEveryWritableField(t *testing.T) {
	workspace, model, title, favorite := "/workspace/new", "deep", "Renamed", true
	stub := sessionCatalogStub{update: func(request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		if request.SessionID != "ses_1" || request.ExpectedRevision != 7 || request.Title == nil || *request.Title != title ||
			request.Workspace == nil || request.Workspace.Path != workspace || request.Model == nil || *request.Model != model ||
			request.Favorite == nil || *request.Favorite != favorite {
			t.Fatalf("update request = %+v", request)
		}
		return &protocol.Session{
			ID: request.SessionID, Title: title, Status: protocol.SessionStatusIdle, Model: model,
			Workspace: protocol.WorkspaceInfo{Ref: *request.Workspace}, Favorite: favorite, Revision: 8,
		}, nil
	}}
	runtime := &Runtime{sessionCatalog: stub, meta: requestMeta("test")}
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Workspace != workspace || updated.Model != model || !updated.Favorite || updated.Revision != 8 {
		t.Fatalf("updated session = %+v", updated)
	}
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
