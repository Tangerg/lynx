package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestUseApplyPatchMatchesNativeAgentDialects(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		want     bool
	}{
		{name: "Codex modern GPT", provider: "openai", model: "gpt-5.6-codex", want: true},
		{name: "Grok", provider: "xai", model: "grok-4.5", want: true},
		{name: "provider-independent GPT dialect", provider: "openrouter", model: "openai/gpt-5", want: true},
		{name: "legacy GPT-4", provider: "openai", model: "gpt-4.1"},
		{name: "OpenAI OSS", provider: "openai", model: "gpt-oss-120b"},
		{name: "Claude", provider: "anthropic", model: "claude-opus-4-1"},
		{name: "Kimi", provider: "moonshot", model: "kimi-k2"},
		{name: "runtime default not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := modelref.New(tt.provider, tt.model)
			if err != nil {
				t.Fatal(err)
			}
			if got := useApplyPatch(selection); got != tt.want {
				t.Fatalf("useApplyPatch(%q, %q) = %v, want %v", tt.provider, tt.model, got, tt.want)
			}
		})
	}
}

func TestResolverRegistersExactlyOneMutationVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		model    string
		patch    bool
	}{
		{name: "Codex", provider: "openai", model: "gpt-5", patch: true},
		{name: "Claude", provider: "anthropic", model: "claude-sonnet", patch: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selection, err := modelref.New(tc.provider, tc.model)
			if err != nil {
				t.Fatal(err)
			}
			built, err := Build(t.Context(), BuildConfig{
				Workdir: t.TempDir(), UserHome: t.TempDir(), DefaultModel: selection,
			})
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
			names := toolNameSet(resolved)
			if tc.patch {
				if !names["apply_patch"] || names["edit"] || names["write"] {
					t.Fatalf("Codex mutation vocabulary = %v", names)
				}
				return
			}
			if names["apply_patch"] || !names["edit"] || !names["write"] {
				t.Fatalf("edit/write mutation vocabulary = %v", names)
			}
		})
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
		Online:         []toolcontract.Tool{named("web_fetch")},
		A2A:            []toolcontract.Tool{named("remote_agent")},
		LSP:            []toolcontract.Tool{named("lsp")},
		Shell:          []toolcontract.Tool{named("shell")},
		AskUser:        named("ask_user"),
		EnterPlan:      named("enter_plan_mode"),
		ExitPlan:       named("exit_plan_mode"),
		Plan:           named("set_plan"),
		ScheduleTools: []toolcontract.Tool{
			named("list_schedules"), named("create_schedule"), named("delete_schedule"),
		},
		ToolResult:    named("read_tool_result"),
		MemorySearch:  named("search_memory"),
		SessionSearch: named("search_conversations"),
		GoalGet:       named("get_goal"),
		ProposeSkill:  named("propose_skill"),
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
	registered := toolNameSet(resolved)
	manifest, err := toolloop.Advertise(resolved)
	if err != nil {
		t.Fatalf("Advertise: %v", err)
	}
	advertised := make(map[string]bool, len(manifest))
	for _, definition := range manifest {
		advertised[definition.Name] = true
	}
	for _, name := range []string{
		"read", "glob", "grep", "edit", "write", "shell", "ask_user",
		"enter_plan_mode", "exit_plan_mode", "set_plan", "read_tool_result",
		"get_goal", "search_tools",
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
