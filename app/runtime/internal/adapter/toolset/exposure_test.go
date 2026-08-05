package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestResolverRegistersExactlyOneMutationVocabulary(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	group, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupRoot)
	if err != nil || !ok {
		t.Fatalf("Resolve = (%v, %v)", ok, err)
	}
	resolved, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	names := definitionNames(resolved)
	if !names[catalog.ApplyPatch] || names["edit"] || names["write"] {
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
	analyzer := codeintel.New(nil)
	t.Cleanup(func() { _ = analyzer.Close() })
	resolver, err := newResolver(resolverDeps{
		DefaultWorkdir: t.TempDir(),
		Online:         []toolcontract.Tool{named(catalog.WebFetch)},
		A2A:            []toolcontract.Tool{named("remote_agent")},
		LSP:            []toolcontract.Tool{named(catalog.LSP)},
		Shell:          []toolcontract.Tool{named(catalog.Shell)},
		AskUser:        named(catalog.AskUser),
		EnterPlan:      named(catalog.EnterPlanMode),
		ExitPlan:       named(catalog.ExitPlanMode),
		Plan:           named(catalog.SetPlan),
		ScheduleTools: []toolcontract.Tool{
			named(catalog.ListSchedules), named(catalog.CreateSchedule), named(catalog.DeleteSchedule),
		},
		ToolResult:    named(catalog.ReadToolResult),
		MemorySearch:  named(catalog.SearchMemory),
		SessionSearch: named(catalog.SearchConversations),
		GoalGet:       named(catalog.GetGoal),
		ProposeSkill:  named(catalog.ProposeSkill),
		CodeIntel:     analyzer,
		ReadTracker:   newReadTracker(),
	})
	if err != nil {
		t.Fatalf("newResolver: %v", err)
	}
	resolver.SetMCPTools([]toolcontract.Tool{
		mcpToolStub{name: "linear_create_issue", server: "linear", remote: "create_issue"},
	})
	group, ok, err := resolver.Resolve(t.Context(), domaintool.GroupRoot)
	if err != nil || !ok {
		t.Fatalf("Resolve = (%v, %v)", ok, err)
	}
	resolved, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	registered := definitionNames(resolved)
	manifest, err := toolloop.InitialManifest(resolved)
	if err != nil {
		t.Fatalf("InitialManifest: %v", err)
	}
	advertised := make(map[string]bool, len(manifest))
	for _, definition := range manifest {
		advertised[definition.Name] = true
	}
	for _, name := range []string{
		catalog.Read, catalog.Glob, catalog.Grep, catalog.ApplyPatch, catalog.Shell, catalog.AskUser,
		catalog.EnterPlanMode, catalog.ExitPlanMode, catalog.SetPlan, catalog.ReadToolResult,
		catalog.GetGoal, catalog.SearchTools,
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

func TestBuildRequiresExplicitProcessPaths(t *testing.T) {
	if _, err := Build(t.Context(), BuildConfig{UserHome: t.TempDir()}); err == nil {
		t.Fatal("Build accepted an empty workdir")
	}
	if _, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir()}); err == nil {
		t.Fatal("Build accepted an empty user home")
	}
	if _, err := Build(t.Context(), BuildConfig{Workdir: "relative", UserHome: t.TempDir()}); err == nil {
		t.Fatal("Build accepted a relative workdir")
	}
	if _, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir(), UserHome: "relative"}); err == nil {
		t.Fatal("Build accepted a relative user home")
	}
}
