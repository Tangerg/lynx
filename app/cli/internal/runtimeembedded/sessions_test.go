package runtimeembedded

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type sessionCatalogStub struct {
	pages  map[string]*protocol.Page[protocol.Session]
	update func(protocol.UpdateSessionRequest) (*protocol.Session, error)
	delete func(protocol.DeleteSessionRequest, embedded.CommandOptions) error
}

func testProtocolWorkspace(path, projectRoot string, availability protocol.WorkspaceAvailability) protocol.WorkspaceInfo {
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: projectRoot, Availability: availability,
	}
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
			Workspace: testProtocolWorkspace(request.Workspace.Path, "/workspace", protocol.WorkspaceAvailable),
			Favorite:  favorite, Revision: 8,
		}, nil
	}}
	runtime := &Runtime{
		sessionCatalog: stub, meta: requestMeta("test"),
		profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeatureRelocate: {Enabled: true, Stability: runtimeprofile.Stable},
		}},
	}
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Workspace.Path != workspace || updated.Workspace.ProjectRoot != "/workspace" || !updated.Workspace.IsAvailable() ||
		updated.Model != model || !updated.Favorite || updated.Revision != 8 {
		t.Fatalf("updated session = %+v", updated)
	}
}

func TestUpdateSessionRejectsWorkspaceWithoutRelocateCapability(t *testing.T) {
	t.Parallel()
	called := false
	runtime := &Runtime{sessionCatalog: sessionCatalogStub{update: func(protocol.UpdateSessionRequest) (*protocol.Session, error) {
		called = true
		return nil, nil
	}}}
	workspace := "/workspace/new"
	if _, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: "ses_1", Workspace: &workspace, ExpectedRevision: 7,
	}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("UpdateSession error = %v, want ErrIncompatibleRuntime", err)
	}
	if called {
		t.Fatal("workspace update reached the binding without relocate capability")
	}
}

func TestProjectSessionPreservesResolvedWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	projected, err := projectSession(protocol.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle,
		Workspace: testProtocolWorkspace("/repo/work", "/repo", protocol.WorkspaceMissing),
	})
	if err != nil {
		t.Fatal(err)
	}
	if projected.Workspace.Path != "/repo/work" || projected.Workspace.ProjectRoot != "/repo" ||
		projected.Workspace.IsAvailable() {
		t.Fatalf("workspace = %+v", projected.Workspace)
	}
	if !matchesSession(projected, "repo", "") || !matchesSession(projected, "", "/repo/work") {
		t.Fatalf("resolved workspace is not searchable: %+v", projected.Workspace)
	}
}

func TestProjectSessionRejectsIncompleteWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace protocol.WorkspaceInfo
	}{
		{name: "project root", workspace: testProtocolWorkspace("/workspace", "", protocol.WorkspaceAvailable)},
		{name: "availability", workspace: testProtocolWorkspace("/workspace", "/workspace", "")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := projectSession(protocol.Session{ID: "ses_1", Status: protocol.SessionStatusIdle, Workspace: test.workspace})
			if err == nil {
				t.Fatalf("projectSession accepted %+v", test.workspace)
			}
		})
	}
}

func (stub sessionCatalogStub) DeleteSession(_ context.Context, request protocol.DeleteSessionRequest, options embedded.CommandOptions) error {
	if stub.delete != nil {
		return stub.delete(request, options)
	}
	return errors.New("unexpected DeleteSession")
}

func TestDeleteSessionUsesTheDurableMutationIdentity(t *testing.T) {
	t.Parallel()
	commandID := agent.CommandID("cli_11111111111111111111111111111111")
	const namespace = "idp_test"
	called := false
	runtime := &Runtime{sessionCatalog: sessionCatalogStub{delete: func(request protocol.DeleteSessionRequest, options embedded.CommandOptions) error {
		called = true
		if request.SessionID != "ses_1" || options.IdempotencyKey != string(commandID) ||
			options.IdempotencyNamespace != namespace {
			t.Fatalf("delete request = %+v, options = %+v", request, options)
		}
		return nil
	}}, meta: requestMeta("test"), profile: runtimeprofile.Profile{
		Limits: runtimeprofile.Limits{IdempotencyNamespace: namespace},
	}}
	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{CommandID: commandID, SessionID: "ses_1"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("delete session did not reach the runtime binding")
	}
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
	requireRuntimeContractViolation(t, err)
}

func TestSessionCatalogRejectsAStalledCursorAndMutationIdentity(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{sessionCatalog: sessionCatalogStub{
		pages: map[string]*protocol.Page[protocol.Session]{
			"stalled": protocol.NewPageWithCursor([]protocol.Session{}, "stalled"),
		},
		update: func(protocol.UpdateSessionRequest) (*protocol.Session, error) {
			return &protocol.Session{
				ID: "ses_other", Status: protocol.SessionStatusIdle,
				Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
			}, nil
		},
	}, meta: requestMeta("test")}

	_, err := runtime.ListSessions(t.Context(), agent.SessionQuery{Cursor: "stalled"})
	requireRuntimeContractViolation(t, err)
	title := "Renamed"
	_, err = runtime.UpdateSession(t.Context(), agent.UpdateSession{SessionID: "ses_1", Title: &title})
	requireRuntimeContractViolation(t, err)
}

func TestSessionCatalogRejectsInvalidLocalFiltersBeforeCallingRuntime(t *testing.T) {
	t.Parallel()

	runtime := &Runtime{sessionCatalog: sessionCatalogStub{}, meta: requestMeta("test")}
	for _, query := range []agent.SessionQuery{
		{Limit: -1},
		{Workspace: "relative/workspace"},
	} {
		if _, err := runtime.ListSessions(t.Context(), query); err == nil {
			t.Fatalf("ListSessions accepted %+v", query)
		}
	}
}
