package runtimeembedded

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type runCatalogBindingStub struct {
	get  func(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error)
	list func(context.Context, protocol.ListRunsRequest, embedded.CallOptions) (*protocol.Page[protocol.RunRef], error)
}

func (stub runCatalogBindingStub) GetRun(ctx context.Context, request protocol.GetRunRequest, options embedded.CallOptions) (*protocol.RunRef, error) {
	return stub.get(ctx, request, options)
}

func (stub runCatalogBindingStub) ListRuns(ctx context.Context, request protocol.ListRunsRequest, options embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
	return stub.list(ctx, request, options)
}

func TestRunCatalogMapsQueriesAndProjectsPages(t *testing.T) {
	t.Parallel()
	wantRun := protocol.RunRef{
		RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Provider: "deepseek", Model: "deepseek-chat",
			Status: protocol.RunStatusFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted},
		},
		ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
	}
	stub := runCatalogBindingStub{
		get: func(_ context.Context, request protocol.GetRunRequest, options embedded.CallOptions) (*protocol.RunRef, error) {
			if request.RunID != wantRun.ID || options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
				t.Fatalf("get = (%+v, %+v)", request, options)
			}
			return &wantRun, nil
		},
		list: func(_ context.Context, request protocol.ListRunsRequest, options embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			if request.SessionID != "ses_1" || len(request.Statuses) != 1 || request.Statuses[0] != protocol.RunStatusFinished ||
				!request.IncludeDescendants || request.Cursor != "opaque" || request.Limit != maximumRunPageSize ||
				options.RequestMeta.ProtocolVersion != protocol.ProtocolVersion {
				t.Fatalf("list = (%+v, %+v)", request, options)
			}
			return protocol.NewPageWithCursor([]protocol.RunRef{wantRun}, "next"), nil
		},
	}
	runtime := &Runtime{
		runCatalog: stub, meta: requestMeta("test"),
		profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeatureSubagents: {
				Enabled: true, Stability: runtimeprofile.Stable, ClientOptIn: true, ClientRequested: true,
			},
		}},
	}
	got, err := runtime.GetRun(t.Context(), "run_1")
	if err != nil || got.ID != "run_1" || got.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("GetRun = %+v, %v", got, err)
	}
	page, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		SessionID: "ses_1", Statuses: []agent.RunStatus{agent.RunStatusFinished},
		IncludeDescendants: true, Cursor: "opaque", Limit: maximumRunPageSize + 20,
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "run_1" || page.NextCursor != "next" {
		t.Fatalf("ListRuns = %+v, %v", page, err)
	}
}

func TestRunCatalogRejectsDescendantQueryWithoutNegotiatedSubagents(t *testing.T) {
	t.Parallel()
	called := false
	runtime := &Runtime{runCatalog: runCatalogBindingStub{
		list: func(context.Context, protocol.ListRunsRequest, embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			called = true
			return protocol.NewPage([]protocol.RunRef{}), nil
		},
	}}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{IncludeDescendants: true}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("ListRuns error = %v, want ErrIncompatibleRuntime", err)
	}
	if called {
		t.Fatal("descendant query reached the binding without negotiated subagents")
	}
}

func TestRunCatalogRejectsIncompleteBindingResults(t *testing.T) {
	t.Parallel()
	stub := runCatalogBindingStub{
		get: func(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error) {
			return nil, nil
		},
		list: func(context.Context, protocol.ListRunsRequest, embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			return nil, nil
		},
	}
	runtime := &Runtime{runCatalog: stub, meta: requestMeta("test")}
	if _, err := runtime.GetRun(t.Context(), "run_1"); err == nil {
		t.Fatal("GetRun accepted nil response")
	} else {
		requireRuntimeContractViolation(t, err)
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{}); err == nil {
		t.Fatal("ListRuns accepted nil response")
	} else {
		requireRuntimeContractViolation(t, err)
	}
	if _, err := runtime.GetRun(t.Context(), " "); err == nil {
		t.Fatal("GetRun accepted empty id")
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{Statuses: []agent.RunStatus{"paused"}}); err == nil {
		t.Fatal("ListRuns accepted invalid status")
	}

	failing := runCatalogBindingStub{
		get: func(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error) {
			return nil, protocol.ErrRunNotFound
		},
		list: func(context.Context, protocol.ListRunsRequest, embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			return nil, protocol.ErrSessionNotFound
		},
	}
	runtime.runCatalog = failing
	if _, err := runtime.GetRun(t.Context(), "missing"); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("GetRun error = %v", err)
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: "missing"}); !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("ListRuns error = %v", err)
	}
}

func TestRunCatalogRejectsResponsesOutsideTheRequestedScope(t *testing.T) {
	t.Parallel()
	base := protocol.RunRef{
		RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_other", Status: protocol.RunStatusFinished,
			Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted}},
		ProtocolProfile: protocol.RunProtocolProfile{RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{}},
	}
	wrongIdentity := base
	wrongIdentity.ID = "run_other"
	runtime := &Runtime{runCatalog: runCatalogBindingStub{get: func(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error) {
		return &wrongIdentity, nil
	}}, meta: requestMeta("test")}
	_, err := runtime.GetRun(t.Context(), "run_1")
	requireRuntimeContractViolation(t, err)

	for _, test := range []struct {
		name  string
		query agent.RunQuery
		value protocol.RunRef
	}{
		{name: "session", query: agent.RunQuery{SessionID: "ses_1"}, value: base},
		{name: "status", query: agent.RunQuery{Statuses: []agent.RunStatus{agent.RunStatusRunning}}, value: base},
		{name: "descendant", value: func() protocol.RunRef {
			value := base
			value.SessionID = "ses_1"
			value.SpawnedByItemID, value.ParentRunID, value.RootRunID = "item_1", "run_root", "run_root"
			return value
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := runCatalogBindingStub{list: func(context.Context, protocol.ListRunsRequest, embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
				return protocol.NewPage([]protocol.RunRef{test.value}), nil
			}}
			runtime := &Runtime{runCatalog: stub, meta: requestMeta("test")}
			_, err := runtime.ListRuns(t.Context(), test.query)
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestRunCatalogOmitsAnEmptyStatusFilter(t *testing.T) {
	t.Parallel()
	stub := runCatalogBindingStub{
		get: func(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error) {
			return nil, errors.New("unexpected get")
		},
		list: func(_ context.Context, request protocol.ListRunsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
			if request.Statuses != nil {
				t.Fatalf("statuses = %#v, want absent", request.Statuses)
			}
			if err := request.ValidateWire(); err != nil {
				t.Fatalf("wire query = %v", err)
			}
			return protocol.NewPage([]protocol.RunRef{}), nil
		},
	}
	runtime := &Runtime{runCatalog: stub, meta: requestMeta("test")}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{Statuses: []agent.RunStatus{}}); err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
}
