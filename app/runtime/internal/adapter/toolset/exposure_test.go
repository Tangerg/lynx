package toolset

import (
	"context"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/codeintel"
	domaintool "github.com/Tangerg/scope/app/runtime/internal/domain/tool"
)

func newTestCodeIntel(t *testing.T) *codeintel.Analyzer {
	t.Helper()
	analyzer, err := codeintel.New(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analyzer.Close() })
	return analyzer
}

func TestResolverRegistersExactlyOneMutationVocabulary(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), DefaultCWD: t.TempDir(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	manifest, err := built.Resolver.Manifest(t.Context(), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	names := definitionNames(manifestTools(manifest))
	if !names[domaintool.ApplyPatch] || names["edit"] || names["write"] {
		t.Fatalf("mutation vocabulary = %v, want apply_patch only", names)
	}
}

func TestResolverInitialManifestSeparatesDirectAndDeferredCapabilities(t *testing.T) {
	named := func(name string) toolcontract.Tool {
		t.Helper()
		candidate, err := toolcontract.NewFunc(
			toolcontract.FuncConfig{Name: name, Description: "test " + name},
			func(context.Context, struct{}) (string, error) { return "ok", nil },
		)
		if err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		return candidate
	}
	analyzer := newTestCodeIntel(t)
	resolver, err := newResolver(resolverDeps{
		DefaultCWD: t.TempDir(),
		Online:     []toolcontract.Tool{named(domaintool.WebFetch)},
		A2A:        []toolcontract.Tool{named("remote_agent")},
		LSP:        []toolcontract.Tool{named(domaintool.LSP)},
		Shell:      []toolcontract.Tool{named(domaintool.Shell)},
		AskUser:    named(domaintool.AskUser),
		EnterPlan:  named(domaintool.EnterPlanMode),
		ExitPlan:   named(domaintool.ExitPlanMode),
		Plan:       named(domaintool.SetPlan),
		ScheduleTools: []toolcontract.Tool{
			named(domaintool.ListSchedules), named(domaintool.CreateSchedule), named(domaintool.DeleteSchedule),
		},
		ToolResult:         named(domaintool.ReadToolResult),
		AgentMemorySearch:  named(domaintool.SearchMemory),
		ConversationSearch: named(domaintool.SearchConversations),
		GoalGet:            named(domaintool.GetGoal),
		ProposeSkill:       named(domaintool.ProposeSkill),
		CodeIntel:          analyzer,
		ReadTracker:        newReadTracker(),
	})
	if err != nil {
		t.Fatalf("newResolver: %v", err)
	}
	resolver.SetMCPTools([]toolcontract.Tool{
		mcpToolStub{name: "linear_create_issue", server: "linear", remote: "create_issue"},
	})
	manifest, err := resolver.Manifest(t.Context(), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	registered := definitionNames(manifestTools(manifest))
	advertised := definitionNames(manifest.Visible)
	for _, name := range []string{
		domaintool.Read, domaintool.Glob, domaintool.Grep, domaintool.ApplyPatch, domaintool.Shell, domaintool.AskUser,
		domaintool.EnterPlanMode, domaintool.ExitPlanMode, domaintool.SetPlan, domaintool.ReadToolResult,
		domaintool.GetGoal, domaintool.SearchTools,
	} {
		if !advertised[name] {
			t.Errorf("direct tool %q missing from initial manifest: %v", name, advertised)
		}
	}
	for _, name := range []string{
		"web_fetch", "remote_agent", "lsp", "linear_create_issue", "list_schedules",
		"create_schedule", "delete_schedule",
		"search_memory", "search_conversations", "propose_skill",
	} {
		if !registered[name] {
			t.Errorf("deferred tool %q missing from Run registry: %v", name, registered)
		}
		if advertised[name] {
			t.Errorf("deferred tool %q leaked into initial manifest: %v", name, advertised)
		}
	}
}

func manifestTools(manifest Manifest) []toolcontract.Tool {
	return append(append([]toolcontract.Tool(nil), manifest.Visible...), manifest.Deferred...)
}

func TestBuildRequiresExplicitProcessPaths(t *testing.T) {
	validPaths := BuildConfig{Lifetime: t.Context(), DefaultCWD: t.TempDir(), UserHome: t.TempDir()}
	var missingContext context.Context
	if _, err := Build(missingContext, validPaths); err == nil {
		t.Fatal("Build accepted a nil startup context")
	}
	validPaths.Lifetime = nil
	if _, err := Build(t.Context(), validPaths); err == nil {
		t.Fatal("Build accepted a nil process lifetime")
	}
	if _, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()}); err == nil {
		t.Fatal("Build accepted an empty default CWD")
	}
	if _, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), DefaultCWD: t.TempDir()}); err == nil {
		t.Fatal("Build accepted an empty user home")
	}
	if _, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), DefaultCWD: "relative", UserHome: t.TempDir()}); err == nil {
		t.Fatal("Build accepted a relative default CWD")
	}
	if _, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), DefaultCWD: t.TempDir(), UserHome: "relative"}); err == nil {
		t.Fatal("Build accepted a relative user home")
	}
}
