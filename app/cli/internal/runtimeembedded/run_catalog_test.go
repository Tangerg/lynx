package runtimeembedded

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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
	runtime := &Runtime{runCatalog: stub, meta: requestMeta("test")}
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
	}
	if _, err := runtime.ListRuns(t.Context(), agent.RunQuery{}); err == nil {
		t.Fatal("ListRuns accepted nil response")
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
