package toolsearch_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/toolsearch"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type mcpTool struct {
	name, desc, server, remote, result string
}

func (t mcpTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        t.name,
		Description: t.desc,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (t mcpTool) Call(context.Context, string) (string, error) {
	if t.result == "" {
		return "ok", nil
	}
	return t.result, nil
}

func (t mcpTool) MCPToolIdentity() (string, string) { return t.server, t.remote }

func catalog() []tools.Tool {
	return []tools.Tool{
		mcpTool{name: "linear_create_issue", desc: "Create a Linear issue", server: "linear", remote: "create_issue"},
		mcpTool{name: "linear_list_issues", desc: "List Linear issues", server: "linear", remote: "list_issues"},
		mcpTool{name: "slack_send_message", desc: "Send a Slack message", server: "slack", remote: "send_message"},
		mcpTool{name: "github_open_pr", desc: "Open a GitHub pull request", server: "github", remote: "open_pr"},
	}
}

func call(t *testing.T, tool *toolsearch.Tool, query string) string {
	t.Helper()
	args, _ := json.Marshal(map[string]any{"query": query})
	out, err := tool.Call(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Call(%q): %v", query, err)
	}
	return out
}

func TestNewEmptyReturnsNil(t *testing.T) {
	if toolsearch.New(nil) != nil {
		t.Fatal("New(nil) should return a nil tool — nothing to search")
	}
}

func TestKeywordSearchRanksByName(t *testing.T) {
	tool := toolsearch.New(catalog())
	out := call(t, tool, "create issue")
	if !strings.Contains(out, "linear_create_issue") {
		t.Fatalf("expected linear_create_issue loaded, got:\n%s", out)
	}
	if strings.Contains(out, "slack_send_message") {
		t.Fatalf("unrelated tool loaded, got:\n%s", out)
	}
}

func TestRequiredTermExcludesNonMatches(t *testing.T) {
	tool := toolsearch.New(catalog())
	out := call(t, tool, "+slack message")
	if !strings.Contains(out, "slack_send_message") {
		t.Fatalf("expected slack_send_message, got:\n%s", out)
	}
	if strings.Contains(out, "linear") || strings.Contains(out, "github") {
		t.Fatalf("+slack should exclude non-slack tools, got:\n%s", out)
	}
}

func TestSelectByExactName(t *testing.T) {
	tool := toolsearch.New(catalog())
	out := call(t, tool, "select:github_open_pr,linear_list_issues")
	if !strings.Contains(out, "github_open_pr") || !strings.Contains(out, "linear_list_issues") {
		t.Fatalf("select did not load both named tools:\n%s", out)
	}
	if strings.Contains(out, "slack_send_message") {
		t.Fatalf("select loaded an unnamed tool:\n%s", out)
	}
}

func TestSelectDropsUnknownNames(t *testing.T) {
	tool := toolsearch.New(catalog())
	out := call(t, tool, "select:does_not_exist")
	if !strings.Contains(out, "No tools matched") {
		t.Fatalf("unknown select should report no match, got:\n%s", out)
	}
}

// TestRoundRobinSpreadsAcrossServers: a query that matches every tool equally
// well must not let one server monopolize the (limited) result window.
func TestRoundRobinSpreadsAcrossServers(t *testing.T) {
	// Six tools, three each on two servers, all matching "issue".
	many := []tools.Tool{
		mcpTool{name: "alpha_issue_a", desc: "issue", server: "alpha", remote: "a"},
		mcpTool{name: "alpha_issue_b", desc: "issue", server: "alpha", remote: "b"},
		mcpTool{name: "alpha_issue_c", desc: "issue", server: "alpha", remote: "c"},
		mcpTool{name: "beta_issue_a", desc: "issue", server: "beta", remote: "a"},
		mcpTool{name: "beta_issue_b", desc: "issue", server: "beta", remote: "b"},
		mcpTool{name: "beta_issue_c", desc: "issue", server: "beta", remote: "c"},
	}
	tool := toolsearch.New(many)
	args, _ := json.Marshal(map[string]any{"query": "issue", "limit": 2})
	out, err := tool.Call(context.Background(), string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "alpha_") || !strings.Contains(out, "beta_") {
		t.Fatalf("limit=2 over two servers should include both servers, got:\n%s", out)
	}
}

func TestDeferredToolNames(t *testing.T) {
	tool := toolsearch.New(catalog())
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
	tool := toolsearch.New(catalog())
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
	tool := toolsearch.New(catalog())
	_, err := tool.Call(context.Background(), `{"query":"  "}`)
	if err == nil {
		t.Fatal("blank query should error so the model retries")
	}
}

// TestEndToEndPromotionMakesWithheldToolCallable runs search_tools through a real
// tool loop: the withheld MCP tool is resolvable but unadvertised; after the
// model searches, the promotion must advertise it so round 2 can call it.
func TestEndToEndPromotionMakesWithheldToolCallable(t *testing.T) {
	withheld := mcpTool{name: "linear_create_issue", desc: "Create a Linear issue", server: "linear", remote: "create_issue", result: "LIN-42 created"}
	search := toolsearch.New([]tools.Tool{withheld})

	registry, err := tools.NewRegistry(search, withheld)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// Manifest advertises ONLY search_tools; the withheld tool is resolvable but
	// not advertised — exactly the resolver + turn projection.
	request := &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("file a linear issue"))},
		Tools:    []chat.ToolDefinition{search.Definition()},
	}

	model := &scriptedModel{call: func(round int, req *chat.Request) (*chat.Response, error) {
		switch round {
		case 1:
			args, _ := json.Marshal(map[string]any{"query": "create issue"})
			return toolResponse(chat.ToolCall{ID: "c1", Name: "search_tools", Arguments: string(args)}), nil
		case 2:
			if !advertises(req.Tools, "linear_create_issue") {
				t.Fatalf("round 2 manifest lacks promoted tool: %v", names(req.Tools))
			}
			return toolResponse(chat.ToolCall{ID: "c2", Name: "linear_create_issue", Arguments: `{}`}), nil
		default:
			return textResponse("done"), nil
		}
	}}

	runner, err := toolloop.NewRunner(model, toolloop.Config{})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	var executed bool
	for event, err := range runner.Run(context.Background(), request, registry) {
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if event.Kind == toolloop.EventToolResult && event.ToolResult != nil &&
			event.ToolResult.Name == "linear_create_issue" && event.ToolResult.Result == "LIN-42 created" {
			executed = true
		}
	}
	if !executed {
		t.Fatal("withheld tool was never executed after search+promotion")
	}
}

type scriptedModel struct {
	calls int
	call  func(int, *chat.Request) (*chat.Response, error)
}

func (m *scriptedModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.calls++
	return m.call(m.calls, request)
}

func toolResponse(calls ...chat.ToolCall) *chat.Response {
	parts := make([]chat.Part, len(calls))
	for i := range calls {
		parts[i] = chat.NewToolCallPart(calls[i])
	}
	msg := chat.NewAssistantMessage(parts...)
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &msg, FinishReason: chat.FinishReasonToolCalls}}}
}

func textResponse(text string) *chat.Response {
	msg := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &msg, FinishReason: chat.FinishReasonStop}}}
}

func advertises(defs []chat.ToolDefinition, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func names(defs []chat.ToolDefinition) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}
