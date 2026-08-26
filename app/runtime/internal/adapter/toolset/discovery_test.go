package toolset_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/core/tool"
)

type mcpTool struct {
	name, desc, server, remote, result string
}

func (m mcpTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        m.name,
		Description: m.desc,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (m mcpTool) Call(context.Context, string) (string, error) {
	if m.result == "" {
		return "ok", nil
	}
	return m.result, nil
}

func (m mcpTool) MCPToolIdentity() (string, string) { return m.server, m.remote }

func catalog() []toolcontract.Tool {
	return []toolcontract.Tool{
		mcpTool{name: "linear_create_issue", desc: "Create a Linear issue", server: "linear", remote: "create_issue"},
		mcpTool{name: "linear_list_issues", desc: "List Linear issues", server: "linear", remote: "list_issues"},
		mcpTool{name: "slack_send_message", desc: "Send a Slack message", server: "slack", remote: "send_message"},
		mcpTool{name: "github_open_pr", desc: "Open a GitHub pull request", server: "github", remote: "open_pr"},
	}
}

func call(t *testing.T, tool *toolset.Discovery, query string) string {
	t.Helper()
	args, _ := json.Marshal(map[string]any{"query": query})
	ctx := toolset.WithToolAdvertiser(context.Background(), func(...string) error { return nil })
	out, err := tool.Call(ctx, string(args))
	if err != nil {
		t.Fatalf("Call(%q): %v", query, err)
	}
	return out
}

func newSearch(t *testing.T, tools []toolcontract.Tool) *toolset.Discovery {
	t.Helper()
	search, err := toolset.NewDiscovery(tools)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return search
}

func TestNewEmptyReturnsNil(t *testing.T) {
	search, err := toolset.NewDiscovery(nil)
	if err != nil {
		t.Fatalf("New(nil): %v", err)
	}
	if search != nil {
		t.Fatal("New(nil) should return a nil tool — nothing to search")
	}
}

func TestKeywordSearchRanksByName(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "create issue")
	if !strings.Contains(out, "linear_create_issue") {
		t.Fatalf("expected linear_create_issue loaded, got:\n%s", out)
	}
	if strings.Contains(out, "slack_send_message") {
		t.Fatalf("unrelated tool loaded, got:\n%s", out)
	}
}

func TestRequiredTermExcludesNonMatches(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "+slack message")
	if !strings.Contains(out, "slack_send_message") {
		t.Fatalf("expected slack_send_message, got:\n%s", out)
	}
	if strings.Contains(out, "linear") || strings.Contains(out, "github") {
		t.Fatalf("+slack should exclude non-slack tools, got:\n%s", out)
	}
}

func TestSelectByExactName(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "select:github_open_pr,linear_list_issues")
	if !strings.Contains(out, "github_open_pr") || !strings.Contains(out, "linear_list_issues") {
		t.Fatalf("select did not load both named tools:\n%s", out)
	}
	if strings.Contains(out, "slack_send_message") {
		t.Fatalf("select loaded an unnamed tool:\n%s", out)
	}
}

func TestSelectDropsUnknownNames(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "select:does_not_exist")
	if !strings.Contains(out, "No tools matched") {
		t.Fatalf("unknown select should report no match, got:\n%s", out)
	}
}

// TestRoundRobinSpreadsAcrossServers: a query that matches every tool equally
// well must not let one server monopolize the (limited) result window.
func TestRoundRobinSpreadsAcrossServers(t *testing.T) {
	// Six tools, three each on two servers, all matching "issue".
	many := []toolcontract.Tool{
		mcpTool{name: "alpha_issue_a", desc: "issue", server: "alpha", remote: "a"},
		mcpTool{name: "alpha_issue_b", desc: "issue", server: "alpha", remote: "b"},
		mcpTool{name: "alpha_issue_c", desc: "issue", server: "alpha", remote: "c"},
		mcpTool{name: "beta_issue_a", desc: "issue", server: "beta", remote: "a"},
		mcpTool{name: "beta_issue_b", desc: "issue", server: "beta", remote: "b"},
		mcpTool{name: "beta_issue_c", desc: "issue", server: "beta", remote: "c"},
	}
	tool := newSearch(t, many)
	args, _ := json.Marshal(map[string]any{"query": "issue", "limit": 2})
	ctx := toolset.WithToolAdvertiser(context.Background(), func(...string) error { return nil })
	out, err := tool.Call(ctx, string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "alpha_") || !strings.Contains(out, "beta_") {
		t.Fatalf("limit=2 over two servers should include both servers, got:\n%s", out)
	}
}

func TestDeferredToolNames(t *testing.T) {
	tool := newSearch(t, catalog())
	names := tool.DeferredToolNames()
	if len(names) != 4 {
		t.Fatalf("DeferredToolNames = %v, want 4 names", names)
	}
	// Mutating the returned slice must not corrupt internal state.
	names[0] = "mutated"
	if tool.DeferredToolNames()[0] == "mutated" {
		t.Fatal("DeferredToolNames leaked internal slice")
	}
}

func TestDescriptionListsCatalogButNotSchemas(t *testing.T) {
	tool := newSearch(t, catalog())
	desc := tool.Definition().Description
	for _, name := range []string{"linear_create_issue", "slack_send_message", "github_open_pr"} {
		if !strings.Contains(desc, name) {
			t.Fatalf("description missing %q:\n%s", name, desc)
		}
	}
	if strings.Contains(desc, "input_schema") || strings.Contains(desc, `"type":"object"`) {
		t.Fatalf("description leaked schemas (defeats deferral):\n%s", desc)
	}
}

func TestEmptyQueryErrors(t *testing.T) {
	tool := newSearch(t, catalog())
	_, err := tool.Call(context.Background(), `{"query":"  "}`)
	if err == nil {
		t.Fatal("blank query should error so the model retries")
	}
}

func TestArgumentsAreStrictAndBounded(t *testing.T) {
	tool := newSearch(t, catalog())
	for _, arguments := range []string{
		`{"query":"issue","unknown":true}`,
		`{"query":"issue","limit":21}`,
		`{}`,
	} {
		if _, err := tool.Call(context.Background(), arguments); err == nil {
			t.Errorf("Call(%s) succeeded, want contract validation error", arguments)
		}
	}
}

func TestAdvertisementUsesExactFrozenToolNames(t *testing.T) {
	search := newSearch(t, catalog())
	var advertised []string
	ctx := toolset.WithToolAdvertiser(context.Background(), func(names ...string) error {
		advertised = append(advertised, names...)
		return nil
	})
	if _, err := search.Call(ctx, `{"query":"select:github_open_pr,linear_list_issues"}`); err != nil {
		t.Fatal(err)
	}
	if strings.Join(advertised, ",") != "github_open_pr,linear_list_issues" {
		t.Fatalf("advertised = %v", advertised)
	}
}

func TestAdvertisementFailsClosedWithoutExecutionBinding(t *testing.T) {
	search := newSearch(t, catalog())
	if _, err := search.Call(context.Background(), `{"query":"create issue"}`); err == nil {
		t.Fatal("search_tools claimed a loaded Tool without an execution advertiser")
	}
}
