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
	create func(protocol.CreateSessionRequest) (*protocol.Session, error)
	update func(protocol.UpdateSessionRequest) (*protocol.Session, error)
	fork   func(protocol.ForkSessionRequest) (*protocol.Session, error)
	delete func(protocol.DeleteSessionRequest, embedded.CommandOptions) error
}

func testProtocolWorkspace(path, projectRoot string, availability protocol.WorkspaceAvailability) protocol.WorkspaceInfo {
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: projectRoot, Availability: availability,
	}
}

const (
	testSessionProvider = "mock"
	testSessionModel    = "balanced"
)

func (stub sessionCatalogStub) ListSessions(_ context.Context, query protocol.PageQuery, _ embedded.CallOptions) (*protocol.Page[protocol.Session], error) {
	return stub.pages[query.Cursor], nil
}

func (stub sessionCatalogStub) CreateSession(_ context.Context, request protocol.CreateSessionRequest, _ embedded.CommandOptions) (*protocol.Session, error) {
	if stub.create != nil {
		return stub.create(request)
	}
	return nil, errors.New("unexpected CreateSession")
}

func (stub sessionCatalogStub) UpdateSession(_ context.Context, request protocol.UpdateSessionRequest, _ embedded.CommandOptions) (*protocol.Session, error) {
	if stub.update != nil {
		return stub.update(request)
	}
	return nil, errors.New("unexpected UpdateSession")
}

func (stub sessionCatalogStub) ForkSession(_ context.Context, request protocol.ForkSessionRequest, _ embedded.CommandOptions) (*protocol.Session, error) {
	if stub.fork != nil {
		return stub.fork(request)
	}
	return nil, errors.New("unexpected ForkSession")
}

func TestCreateAndForkSessionRejectAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	base := protocol.Session{
		ID: "ses_new", Title: "Requested", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
		Revision:  1,
	}
	tests := []struct {
		name    string
		invoke  func(*Runtime) error
		binding sessionCatalogStub
	}{
		{
			name: "create title",
			binding: sessionCatalogStub{create: func(protocol.CreateSessionRequest) (*protocol.Session, error) {
				result := base
				result.Title = "Ignored"
				return &result, nil
			}},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: base.Title, Workspace: "/workspace"})
				return err
			},
		},
		{
			name: "create workspace",
			binding: sessionCatalogStub{create: func(protocol.CreateSessionRequest) (*protocol.Session, error) {
				result := base
				result.Workspace = testProtocolWorkspace("/other", "/other", protocol.WorkspaceAvailable)
				return &result, nil
			}},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: base.Title, Workspace: "/workspace"})
				return err
			},
		},
		{
			name: "fork title",
			binding: sessionCatalogStub{fork: func(protocol.ForkSessionRequest) (*protocol.Session, error) {
				result := base
				result.Title = "Ignored"
				return &result, nil
			}},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_source", Title: base.Title})
				return err
			},
		},
		{
			name: "fork source identity",
			binding: sessionCatalogStub{fork: func(request protocol.ForkSessionRequest) (*protocol.Session, error) {
				result := base
				result.ID = request.SessionID
				return &result, nil
			}},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_source", Title: base.Title})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{
				sessionCatalog: test.binding,
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: "/workspace"},
					ProjectRoot:  "/workspace",
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
			}
			requireRuntimeContractViolation(t, test.invoke(runtime))
		})
	}
}

func TestUpdateSessionProjectsEveryWritableField(t *testing.T) {
	workspace, title, favorite := "/workspace/new", "Renamed", true
	model := agent.ModelRef{Provider: "deepseek", Model: "deep"}
	stub := sessionCatalogStub{update: func(request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		if request.SessionID != "ses_1" || request.ExpectedRevision != 7 || request.Title == nil || *request.Title != title ||
			request.Workspace == nil || request.Workspace.Path != workspace || request.Provider == nil || *request.Provider != model.Provider ||
			request.Model == nil || *request.Model != model.Model ||
			request.Favorite == nil || *request.Favorite != favorite {
			t.Fatalf("update request = %+v", request)
		}
		return &protocol.Session{
			ID: request.SessionID, Title: title, Status: protocol.SessionStatusIdle, Provider: model.Provider, Model: model.Model,
			Workspace: testProtocolWorkspace(request.Workspace.Path, "/workspace", protocol.WorkspaceAvailable),
			Favorite:  favorite, Revision: 8,
		}, nil
	}}
	runtime := &Runtime{
		sessionCatalog: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: workspace},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
		profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeatureRelocate: {Enabled: true},
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
		updated.Provider != model.Provider || updated.Model != model.Model || !updated.Favorite || updated.Revision != 8 {
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

func TestUpdateSessionRejectsAcknowledgementsThatDidNotApplyTheMutation(t *testing.T) {
	t.Parallel()
	workspace, title, favorite := "/workspace/new", "Renamed", true
	model := agent.ModelRef{Provider: "deepseek", Model: "deep"}
	request := agent.UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: 7,
	}
	valid := protocol.Session{
		ID: request.SessionID, Title: title, Status: protocol.SessionStatusIdle, Provider: model.Provider, Model: model.Model,
		Workspace: testProtocolWorkspace(workspace, "/workspace", protocol.WorkspaceAvailable),
		Favorite:  favorite, Revision: 8,
	}
	tests := []struct {
		name   string
		mutate func(*protocol.Session)
	}{
		{name: "stale revision", mutate: func(session *protocol.Session) { session.Revision = 7 }},
		{name: "title", mutate: func(session *protocol.Session) { session.Title = "Old" }},
		{name: "workspace", mutate: func(session *protocol.Session) { session.Workspace.Ref.Path = "/workspace/old" }},
		{name: "model", mutate: func(session *protocol.Session) { session.Model = "shallow" }},
		{name: "favorite", mutate: func(session *protocol.Session) { session.Favorite = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := valid
			test.mutate(&result)
			runtime := &Runtime{
				sessionCatalog: sessionCatalogStub{update: func(protocol.UpdateSessionRequest) (*protocol.Session, error) {
					return &result, nil
				}},
				workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
					Ref:          protocol.WorkspaceRef{Path: workspace},
					ProjectRoot:  workspace,
					Availability: protocol.WorkspaceAvailable,
				}},
				meta: requestMeta("test"),
				profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
					runtimeprofile.FeatureRelocate: {Enabled: true},
				}},
			}
			_, err := runtime.UpdateSession(t.Context(), request)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestSessionMutationsUseResolvedWorkspaceIdentity(t *testing.T) {
	t.Parallel()
	const requested = "/workspace/alias"
	const canonical = "/workspace/canonical"
	resolved := &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
		Ref:          protocol.WorkspaceRef{Path: canonical},
		ProjectRoot:  canonical,
		Availability: protocol.WorkspaceAvailable,
	}}
	result := protocol.Session{
		ID: "ses_1", Title: "Requested", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testProtocolWorkspace(canonical, canonical, protocol.WorkspaceAvailable),
		Revision:  1,
	}
	catalog := sessionCatalogStub{
		create: func(request protocol.CreateSessionRequest) (*protocol.Session, error) {
			if request.Workspace == nil || request.Workspace.Path != canonical {
				t.Fatalf("create workspace = %+v, want %q", request.Workspace, canonical)
			}
			created := result
			return &created, nil
		},
		update: func(request protocol.UpdateSessionRequest) (*protocol.Session, error) {
			if request.Workspace == nil || request.Workspace.Path != canonical {
				t.Fatalf("update workspace = %+v, want %q", request.Workspace, canonical)
			}
			updated := result
			updated.Revision = 2
			return &updated, nil
		},
	}
	runtime := &Runtime{
		sessionCatalog: catalog, workspaces: resolved, meta: requestMeta("test"),
		profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeatureRelocate: {Enabled: true},
		}},
	}
	if _, err := runtime.CreateSession(t.Context(), agent.CreateSession{
		Title: result.Title, Workspace: requested,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	requestedWorkspace := requested
	if _, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: result.ID, Workspace: &requestedWorkspace, ExpectedRevision: 1,
	}); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
}

func TestProjectSessionPreservesResolvedWorkspaceIdentity(t *testing.T) {
	t.Parallel()

	projected, err := projectSession(protocol.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle,
		Provider: testSessionProvider, Model: testSessionModel,
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
			_, err := projectSession(protocol.Session{ID: "ses_1", Status: protocol.SessionStatusIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: test.workspace})
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
				Provider: testSessionProvider, Model: testSessionModel,
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
