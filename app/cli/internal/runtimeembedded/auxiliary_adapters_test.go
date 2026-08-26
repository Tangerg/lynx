package runtimeembedded

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
	"github.com/Tangerg/lynx/app/cli/internal/feedback"
)

type diagnosticToolBindingStub struct {
	t      *testing.T
	page   *protocol.Page[protocol.ToolSpec]
	result any
}

func (d *diagnosticToolBindingStub) ListTools(_ context.Context, options embedded.CallOptions) (*protocol.Page[protocol.ToolSpec], error) {
	assertCallMeta(d.t, options.RequestMeta)
	return d.page, nil
}

func (d *diagnosticToolBindingStub) InvokeTool(_ context.Context, request protocol.InvokeToolRequest, options embedded.CommandOptions) (any, error) {
	assertCommandMeta(d.t, options)
	if request.Name != "inspect" || request.Workspace == nil || request.Workspace.Path != "/workspace" || request.Arguments["depth"] != float64(2) {
		d.t.Fatalf("tool invocation = %+v", request)
	}
	return d.result, nil
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
			} else {
				requireRuntimeContractViolation(t, err)
			}
		})
	}
}

type authoringContextBindingStub struct {
	t       *testing.T
	docs    *protocol.Page[protocol.AgentDoc]
	recipes *protocol.Page[protocol.Recipe]
}

func (a *authoringContextBindingStub) ListAgentDocs(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.AgentDoc], error) {
	assertWorkspaceQuery(a.t, request, options)
	return a.docs, nil
}

func (a *authoringContextBindingStub) ListRecipes(_ context.Context, request protocol.WorkspaceQuery, options embedded.CallOptions) (*protocol.Page[protocol.Recipe], error) {
	assertWorkspaceQuery(a.t, request, options)
	return a.recipes, nil
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
	t           *testing.T
	workspace   string
	projectRoot string
	result      *protocol.HooksListResult
	trusted     *bool
}

func (h *hookBindingStub) ListHooks(_ context.Context, request protocol.ListHooksRequest, options embedded.CallOptions) (*protocol.HooksListResult, error) {
	assertCallMeta(h.t, options.RequestMeta)
	if request.Workspace.Path != h.workspace {
		h.t.Fatalf("hooks request = %+v", request)
	}
	return h.result, nil
}

func (h *hookBindingStub) SetHookTrust(_ context.Context, request protocol.SetHookTrustRequest, options embedded.CommandOptions) error {
	assertCommandMeta(h.t, options)
	if request.ProjectRoot != h.projectRoot {
		h.t.Fatalf("hook trust request = %+v", request)
	}
	h.trusted = new(request.Trusted)
	return nil
}

func TestHookAndFeedbackAdaptersPreserveGovernanceAndTargeting(t *testing.T) {
	projectRoot := t.TempDir()
	workspace := filepath.Join(projectRoot, "nested")
	hooks := &hookBindingStub{t: t, workspace: workspace, projectRoot: projectRoot, result: &protocol.HooksListResult{
		ProjectRoot: projectRoot, ProjectTrusted: false,
		Hooks: []protocol.HookInfo{{Event: protocol.HookEventPreToolUse, Matcher: "shell*", Command: "check", Scope: protocol.HookScopeProject, Source: filepath.Join(workspace, ".lyra", "hooks.json")}},
	}}
	hookAdapter := &hookAdapter{runtime: &Runtime{hooks: hooks, meta: requestMeta("test")}}
	catalog, err := hookAdapter.Catalog(t.Context(), workspace)
	if err != nil || len(catalog.Hooks) != 1 || catalog.Hooks[0].Active {
		t.Fatalf("Catalog = (%+v, %v)", catalog, err)
	}
	if err := hookAdapter.SetProjectTrust(t.Context(), projectRoot, true); err != nil || hooks.trusted == nil || !*hooks.trusted {
		t.Fatalf("SetProjectTrust = %v, trusted %v", err, hooks.trusted)
	}

	feedbacks := &feedbackBindingStub{t: t}
	feedbackAdapter := &feedbackAdapter{runtime: &Runtime{feedback: feedbacks, meta: requestMeta("test")}}
	signal := feedback.Signal{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", Rating: feedback.Positive, Text: "useful"}
	if err := feedbackAdapter.Record(t.Context(), signal); err != nil || feedbacks.recorded != signal {
		t.Fatalf("Record = %v, recorded %+v", err, feedbacks.recorded)
	}
}

func TestHookAdapterRejectsCatalogForAnotherProject(t *testing.T) {
	workspace := t.TempDir()
	hooks := &hookBindingStub{t: t, workspace: workspace, result: &protocol.HooksListResult{ProjectRoot: t.TempDir()}}
	adapter := &hookAdapter{runtime: &Runtime{hooks: hooks, meta: requestMeta("test")}}
	_, err := adapter.Catalog(t.Context(), workspace)
	requireRuntimeContractViolation(t, err)
}

type feedbackBindingStub struct {
	t        *testing.T
	recorded feedback.Signal
}

func (f *feedbackBindingStub) CreateFeedback(_ context.Context, request protocol.FeedbackRequest, options embedded.CommandOptions) error {
	assertCommandMeta(f.t, options)
	f.recorded = feedback.Signal{
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
