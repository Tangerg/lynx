package runtimeembedded

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
	"github.com/Tangerg/lynx/app/cli/internal/codebase"
	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
	"github.com/Tangerg/lynx/app/cli/internal/feedback"
)

type diagnosticToolBindingStub struct {
	t      *testing.T
	page   *protocol.Page[protocol.ToolSpec]
	result any
}

func (stub *diagnosticToolBindingStub) ListTools(_ context.Context, options embedded.CallOptions) (*protocol.Page[protocol.ToolSpec], error) {
	assertCallMeta(stub.t, options.RequestMeta)
	return stub.page, nil
}

func (stub *diagnosticToolBindingStub) InvokeTool(_ context.Context, request protocol.InvokeToolRequest, options embedded.CommandOptions) (any, error) {
	assertCommandMeta(stub.t, options)
	if request.Name != "inspect" || request.Workspace == nil || request.Workspace.Path != "/workspace" || request.Arguments["depth"] != float64(2) {
		stub.t.Fatalf("tool invocation = %+v", request)
	}
	return stub.result, nil
}

func TestDiagnosticToolAdapterConfinesSafeCatalogAndJSON(t *testing.T) {
	stub := &diagnosticToolBindingStub{t: t, page: protocol.NewPage([]protocol.ToolSpec{{
		Name: "inspect", Description: "inspect state", SafetyClass: protocol.SafetyClassSafe,
		Parameters: map[string]any{"type": "object"},
	}}), result: map[string]any{"ok": true}}
	adapter := &diagnosticToolAdapter{runtime: &Runtime{diagnosticTools: stub, meta: requestMeta("test")}}
	tools, err := adapter.Tools(t.Context())
	if err != nil || len(tools) != 1 || tools[0].Name != "inspect" {
		t.Fatalf("Tools = (%+v, %v)", tools, err)
	}
	result, err := adapter.Invoke(t.Context(), diagnostictool.Invocation{
		Tool: tools[0], Arguments: json.RawMessage(`{"depth":2}`), Workspace: "/workspace",
	})
	if err != nil || string(result.JSON) != `{"ok":true}` {
		t.Fatalf("Invoke = (%s, %v)", result.JSON, err)
	}
}

func TestDiagnosticToolAdapterRejectsUnaddressableOrUnsafeCatalogs(t *testing.T) {
	for name, page := range map[string]*protocol.Page[protocol.ToolSpec]{
		"nil":          nil,
		"continuation": {NextCursor: "next"},
		"unsafe":       protocol.NewPage([]protocol.ToolSpec{{Name: "write", SafetyClass: protocol.SafetyClassWrite, Parameters: map[string]any{}}}),
		"duplicate": protocol.NewPage([]protocol.ToolSpec{
			{Name: "inspect", SafetyClass: protocol.SafetyClassSafe, Parameters: map[string]any{}},
			{Name: "inspect", SafetyClass: protocol.SafetyClassSafe, Parameters: map[string]any{}},
		}),
	} {
		t.Run(name, func(t *testing.T) {
			adapter := &diagnosticToolAdapter{runtime: &Runtime{
				diagnosticTools: &diagnosticToolBindingStub{t: t, page: page}, meta: requestMeta("test"),
			}}
			if _, err := adapter.Tools(t.Context()); err == nil {
				t.Fatal("broken tool catalog was accepted")
			}
		})
	}
}

type codebaseBindingStub struct {
	t           *testing.T
	status      *protocol.CodebaseStatus
	hits        []protocol.CodebaseHit
	operationID string
}

func (stub *codebaseBindingStub) SearchCodebase(_ context.Context, request protocol.CodebaseSearchRequest, options embedded.CallOptions) (*protocol.CodebaseSearchResult, error) {
	assertCallMeta(stub.t, options.RequestMeta)
	if request.Workspace.Path != "/workspace" || request.Query != "ownership" || request.Limit != 8 {
		stub.t.Fatalf("codebase search request = %+v", request)
	}
	return &protocol.CodebaseSearchResult{Hits: stub.hits}, nil
}

func (stub *codebaseBindingStub) GetCodebaseStatus(_ context.Context, request protocol.CodebaseStatusRequest, options embedded.CallOptions) (*protocol.CodebaseStatus, error) {
	assertCallMeta(stub.t, options.RequestMeta)
	if request.Workspace.Path != "/workspace" {
		stub.t.Fatalf("codebase status request = %+v", request)
	}
	return stub.status, nil
}

func (stub *codebaseBindingStub) ReindexCodebase(_ context.Context, request protocol.CodebaseReindexRequest, options embedded.CommandOptions) (*protocol.CodebaseReindexResponse, error) {
	assertCommandMeta(stub.t, options)
	if request.Workspace.Path != "/workspace" {
		stub.t.Fatalf("codebase reindex request = %+v", request)
	}
	return &protocol.CodebaseReindexResponse{OperationID: stub.operationID}, nil
}

func TestCodebaseAdapterProjectsLifecycleSearchAndReindex(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	stub := &codebaseBindingStub{
		t: t, status: &protocol.CodebaseStatus{State: protocol.CodebaseStateReady, ModelID: "embed", FileCount: 2, ChunkCount: 3, IndexedAt: now.Format(time.RFC3339)},
		hits: []protocol.CodebaseHit{{Path: "main.go", StartLine: 2, EndLine: 4, Snippet: "owner", Score: .9}}, operationID: "op_1",
	}
	adapter := &codebaseAdapter{runtime: &Runtime{codebase: stub, meta: requestMeta("test")}}
	status, err := adapter.Status(t.Context(), "/workspace")
	if err != nil || status.IndexedAt == nil || !status.IndexedAt.Equal(now) {
		t.Fatalf("Status = (%+v, %v)", status, err)
	}
	hits, err := adapter.Search(t.Context(), codebase.Query{Workspace: "/workspace", Text: "ownership", Limit: 8})
	if err != nil || len(hits) != 1 || hits[0].Score != .9 {
		t.Fatalf("Search = (%+v, %v)", hits, err)
	}
	operation, err := adapter.Reindex(t.Context(), "/workspace")
	if err != nil || operation.ID != "op_1" {
		t.Fatalf("Reindex = (%+v, %v)", operation, err)
	}
}

func TestCodebaseAdapterRejectsMalformedProjections(t *testing.T) {
	for name, stub := range map[string]*codebaseBindingStub{
		"nil status": {t: t},
		"bad time":   {t: t, status: &protocol.CodebaseStatus{State: protocol.CodebaseStateReady, IndexedAt: "not-time"}},
		"bad hit": {t: t, hits: []protocol.CodebaseHit{{
			Path: "main.go", StartLine: 4, EndLine: 2, Score: .5,
		}}},
		"empty operation": {t: t, operationID: ""},
	} {
		t.Run(name, func(t *testing.T) {
			adapter := &codebaseAdapter{runtime: &Runtime{codebase: stub, meta: requestMeta("test")}}
			switch name {
			case "nil status", "bad time":
				if _, err := adapter.Status(t.Context(), "/workspace"); err == nil {
					t.Fatal("malformed status was accepted")
				}
			case "bad hit":
				if _, err := adapter.Search(t.Context(), codebase.Query{Workspace: "/workspace", Text: "ownership", Limit: 8}); err == nil {
					t.Fatal("malformed hit was accepted")
				}
			case "empty operation":
				if _, err := adapter.Reindex(t.Context(), "/workspace"); err == nil {
					t.Fatal("empty operation was accepted")
				}
			}
		})
	}
}

type authoringContextBindingStub struct {
	t       *testing.T
	docs    *protocol.Page[protocol.AgentDoc]
	recipes *protocol.Page[protocol.Recipe]
}

func (stub *authoringContextBindingStub) ListAgentDocs(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.AgentDoc], error) {
	assertWorkspaceQuery(stub.t, request, options)
	return stub.docs, nil
}

func (stub *authoringContextBindingStub) ListRecipes(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.Recipe], error) {
	assertWorkspaceQuery(stub.t, request, options)
	return stub.recipes, nil
}

func TestAuthoringContextAdapterProjectsDocumentsAndRecipes(t *testing.T) {
	stub := &authoringContextBindingStub{t: t,
		docs: protocol.NewPage([]protocol.AgentDoc{{Path: "/workspace/AGENTS.md", Scope: protocol.AgentDocScopeProjectRoot}}),
		recipes: protocol.NewPage([]protocol.Recipe{{
			Name: "review", Body: "review $ARGUMENTS", Scope: protocol.RecipeScopeProject, Source: "/workspace/.lyra/recipes/review.md",
		}}),
	}
	adapter := &authoringContextAdapter{runtime: &Runtime{authoringContext: stub, meta: requestMeta("test")}}
	documents, err := adapter.Documents(t.Context(), "/workspace")
	if err != nil || len(documents) != 1 || documents[0].Scope != authoringcontext.DocumentProjectRoot {
		t.Fatalf("Documents = (%+v, %v)", documents, err)
	}
	recipes, err := adapter.Recipes(t.Context(), "/workspace")
	if err != nil || len(recipes) != 1 {
		t.Fatalf("Recipes = (%+v, %v)", recipes, err)
	}
}

type hookBindingStub struct {
	t       *testing.T
	result  *protocol.HooksListResult
	trusted *bool
}

func (stub *hookBindingStub) ListHooks(_ context.Context, request protocol.ListHooksRequest, options embedded.CallOptions) (*protocol.HooksListResult, error) {
	assertCallMeta(stub.t, options.RequestMeta)
	if request.Workspace.Path != "/workspace" {
		stub.t.Fatalf("hooks request = %+v", request)
	}
	return stub.result, nil
}

func (stub *hookBindingStub) SetHookTrust(_ context.Context, request protocol.SetHookTrustRequest, options embedded.CommandOptions) error {
	assertCommandMeta(stub.t, options)
	if request.ProjectRoot != "/workspace" {
		stub.t.Fatalf("hook trust request = %+v", request)
	}
	stub.trusted = new(request.Trusted)
	return nil
}

func TestHookAndFeedbackAdaptersPreserveGovernanceAndTargeting(t *testing.T) {
	hooks := &hookBindingStub{t: t, result: &protocol.HooksListResult{
		ProjectRoot: "/workspace", ProjectTrusted: false,
		Hooks: []protocol.HookInfo{{Event: protocol.HookEventPreToolUse, Matcher: "shell*", Command: "check", Scope: protocol.HookScopeProject, Source: "/workspace/.lyra/hooks.json"}},
	}}
	hookAdapter := &hookAdapter{runtime: &Runtime{hooks: hooks, meta: requestMeta("test")}}
	catalog, err := hookAdapter.Catalog(t.Context(), "/workspace")
	if err != nil || len(catalog.Hooks) != 1 || catalog.Hooks[0].Active {
		t.Fatalf("Catalog = (%+v, %v)", catalog, err)
	}
	if err := hookAdapter.SetProjectTrust(t.Context(), "/workspace", true); err != nil || hooks.trusted == nil || !*hooks.trusted {
		t.Fatalf("SetProjectTrust = %v, trusted %v", err, hooks.trusted)
	}

	feedbacks := &feedbackBindingStub{t: t}
	feedbackAdapter := &feedbackAdapter{runtime: &Runtime{feedback: feedbacks, meta: requestMeta("test")}}
	signal := feedback.Signal{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", Rating: feedback.Positive, Text: "useful"}
	if err := feedbackAdapter.Record(t.Context(), signal); err != nil || feedbacks.recorded != signal {
		t.Fatalf("Record = %v, recorded %+v", err, feedbacks.recorded)
	}
}

type feedbackBindingStub struct {
	t        *testing.T
	recorded feedback.Signal
}

func (stub *feedbackBindingStub) CreateFeedback(_ context.Context, request protocol.FeedbackRequest, options embedded.CommandOptions) error {
	assertCommandMeta(stub.t, options)
	stub.recorded = feedback.Signal{
		SessionID: request.SessionID, RunID: request.RunID, ItemID: request.ItemID,
		Rating: feedback.Rating(request.Rating), Text: request.Text,
	}
	return nil
}

func assertWorkspaceQuery(t *testing.T, request protocol.WorkspaceQuery, options embedded.CallOptions) {
	t.Helper()
	assertCallMeta(t, options.RequestMeta)
	if request.Workspace.Path != "/workspace" {
		t.Fatalf("workspace query = %+v", request)
	}
}

func assertCallMeta(t *testing.T, meta protocol.RequestMeta) {
	t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("request meta = %+v", meta)
	}
}

func assertCommandMeta(t *testing.T, options embedded.CommandOptions) {
	t.Helper()
	assertCallMeta(t, options.RequestMeta)
	if options.IdempotencyKey == "" {
		t.Fatal("command has no idempotency key")
	}
}
