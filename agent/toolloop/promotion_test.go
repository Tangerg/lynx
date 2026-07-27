package toolloop_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// TestPromoteToolsAdvertisesWithheldToolMidLoop drives the full seam: a
// search_tools call in round 1 promotes a tool the initial manifest withheld;
// the runner must advertise it on round 2 so the model can call it directly.
func TestPromoteToolsAdvertisesWithheldToolMidLoop(t *testing.T) {
	withheld := newRunnerTool("mcp_create_issue", func(context.Context, string) (string, error) {
		return "issue created", nil
	})
	var promoted bool
	search := newRunnerTool("search_tools", func(ctx context.Context, _ string) (string, error) {
		promoted = true
		toolloop.PromoteTools(ctx, withheld.Definition())
		return "found mcp_create_issue — now callable", nil
	})
	// The registry (resolver) holds BOTH tools so the promoted one is
	// executable; only search_tools is advertised up front.
	registry := newRunnerRegistry(t, search, withheld)
	request := protocolRequest(t)
	request.Tools = []chat.ToolDefinition{search.Definition()}

	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		switch round {
		case 1:
			if len(request.Tools) != 1 || request.Tools[0].Name != "search_tools" {
				t.Fatalf("round 1 manifest = %v, want only search_tools", toolNames(request.Tools))
			}
			return runnerToolResponse(chat.ToolCall{ID: "c1", Name: "search_tools", Arguments: `{}`}), nil
		case 2:
			if !hasTool(request.Tools, "mcp_create_issue") {
				t.Fatalf("round 2 manifest = %v, want promoted mcp_create_issue advertised", toolNames(request.Tools))
			}
			return runnerToolResponse(chat.ToolCall{ID: "c2", Name: "mcp_create_issue", Arguments: `{}`}), nil
		default:
			return runnerTextResponse("done"), nil
		}
	}}

	runner := newRunner(t, model, toolloop.Config{})
	events, err := collectRunnerEvents(runner.Run(context.Background(), request, registry))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !promoted {
		t.Fatal("search_tools was never invoked")
	}
	if !hasToolResult(events, "mcp_create_issue", "issue created") {
		t.Fatal("promoted tool was never executed")
	}
}

// TestPromoteToolsRejectsUnresolvableDefinition guards the advertised⊆resolvable
// invariant: a promoted definition the resolver cannot execute must never enter
// the manifest, or a later round/resume would advertise a phantom tool.
func TestPromoteToolsRejectsUnresolvableDefinition(t *testing.T) {
	phantom := chat.ToolDefinition{
		Name:        "not_in_registry",
		Description: "phantom",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
	search := newRunnerTool("search_tools", func(ctx context.Context, _ string) (string, error) {
		toolloop.PromoteTools(ctx, phantom)
		return "searched", nil
	})
	registry := newRunnerRegistry(t, search)
	request := protocolRequest(t)
	request.Tools = []chat.ToolDefinition{search.Definition()}

	model := &scriptedModel{call: func(round int, request *chat.Request) (*chat.Response, error) {
		switch round {
		case 1:
			return runnerToolResponse(chat.ToolCall{ID: "c1", Name: "search_tools", Arguments: `{}`}), nil
		default:
			if hasTool(request.Tools, "not_in_registry") {
				t.Fatalf("unresolvable tool leaked into manifest: %v", toolNames(request.Tools))
			}
			return runnerTextResponse("done"), nil
		}
	}}

	runner := newRunner(t, model, toolloop.Config{})
	if _, err := collectRunnerEvents(runner.Run(context.Background(), request, registry)); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestPromoteToolsNoOpWithoutRunner confirms a tool that promotes still works
// when invoked outside a running loop (no sink bound): the promotion is silently
// dropped, the call is unaffected.
func TestPromoteToolsNoOpWithoutRunner(t *testing.T) {
	out, err := newRunnerTool("t", func(ctx context.Context, _ string) (string, error) {
		toolloop.PromoteTools(ctx, chat.ToolDefinition{Name: "x"})
		return "ok", nil
	}).Call(context.Background(), `{}`)
	if err != nil || out != "ok" {
		t.Fatalf("call = (%q, %v), want (\"ok\", nil)", out, err)
	}
}

func toolNames(defs []chat.ToolDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func hasTool(defs []chat.ToolDefinition, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func hasToolResult(events []toolloop.Event, name, result string) bool {
	for _, e := range events {
		if e.Kind == toolloop.EventToolResult && e.ToolResult != nil &&
			e.ToolResult.Name == name && e.ToolResult.Result == result {
			return true
		}
	}
	return false
}

type advertiseTool struct {
	name     string
	deferred []string
}

func (t advertiseTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: t.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (advertiseTool) Call(context.Context, string) (string, error) { return "", nil }

func (t advertiseTool) DeferredToolNames() []string { return t.deferred }

// TestAdvertiseWithholdsDeferredNames pins the half of promotion that decides
// what the model sees first: a deferring tool keeps its names out of the
// manifest while they stay resolvable, and everything else is advertised in
// name order.
func TestAdvertiseWithholdsDeferredNames(t *testing.T) {
	search := advertiseTool{name: "search_tools", deferred: []string{"catalog_b", "catalog_a", "absent"}}
	manifest := toolloop.Advertise([]tools.Tool{
		advertiseTool{name: "read"},
		advertiseTool{name: "catalog_a"},
		advertiseTool{name: "catalog_b"},
		search,
	})

	var names []string
	for _, definition := range manifest {
		names = append(names, definition.Name)
	}
	if len(names) != 2 || names[0] != "read" || names[1] != "search_tools" {
		t.Fatalf("manifest = %v, want [read search_tools] in name order", names)
	}
}

// TestAdvertiseKeepsEveryToolWithoutDeferral pins that the projection is
// transparent when no tool withholds anything.
func TestAdvertiseKeepsEveryToolWithoutDeferral(t *testing.T) {
	manifest := toolloop.Advertise([]tools.Tool{
		advertiseTool{name: "write"},
		advertiseTool{name: "read"},
	})
	if len(manifest) != 2 || manifest[0].Name != "read" || manifest[1].Name != "write" {
		t.Fatalf("manifest = %+v, want both tools in name order", manifest)
	}
}
