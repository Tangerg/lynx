package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
)

// Domain types — the agent takes a Topic and produces a Brief by asking
// the LLM (with a search tool wired in).
type (
	Topic struct{ Title string }
	Brief struct {
		Topic   string
		Sources []string
		Summary string
	}
)

func main() {
	chatClient, err := chat.NewClient(newStubModel())
	if err != nil {
		log.Fatal(err)
	}

	resolver := core.NewStaticToolGroupResolver("static")
	resolver.Register("research", newResearchToolGroup())

	a := agent.New("BriefingAgent").
		Description("ask the LLM for a topic brief, with a search tool available").
		Actions(agent.NewAction("brief",
			func(ctx context.Context, pc *core.ProcessContext, in Topic) (Brief, error) {
				req, err := pc.ChatWithActionTools(ctx)
				if err != nil {
					return Brief{}, err
				}

				prompt := fmt.Sprintf(
					"Write a one-paragraph brief on %q. "+
						"Use the `research_search` tool to gather sources first; "+
						"then summarise. Reply with JSON: "+
						`{"summary":"...","sources":["..."]}`,
					in.Title,
				)

				text, _, err := req.
					WithSystemPrompt("You are a research analyst. Cite sources you used.").
					WithUserPrompt(prompt).
					Call().
					Text(ctx)
				if err != nil {
					return Brief{}, err
				}

				var parsed struct {
					Summary string   `json:"summary"`
					Sources []string `json:"sources"`
				}
				if jsonErr := json.Unmarshal([]byte(extractJSON(text)), &parsed); jsonErr != nil {
					// Fall back to plaintext when the LLM didn't follow the JSON cue.
					parsed.Summary = strings.TrimSpace(text)
				}
				return Brief{
					Topic:   in.Title,
					Sources: parsed.Sources,
					Summary: parsed.Summary,
				}, nil
			},
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor("research"),
			},
		)).
		Goals(agent.GoalProducing[Brief](core.Goal{Description: "topic brief produced"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		ChatClient: chatClient,
		Extensions: []core.Extension{resolver, &eventLogger{}},
	})
	if err := platform.Deploy(a); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(), a,
		map[string]any{core.DefaultBindingName: Topic{Title: "agent frameworks in 2026"}},
		core.ProcessOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	brief, ok := core.ResultOfType[Brief](proc)
	if !ok {
		log.Fatalf("no Brief produced; status=%s", proc.Status())
	}

	fmt.Println("\n--- result ---")
	fmt.Printf("topic:   %s\n", brief.Topic)
	fmt.Printf("sources: %v\n", brief.Sources)
	fmt.Printf("summary: %s\n", brief.Summary)
}

// ============================================================================
// Stub LLM — pretends to use the search tool, then emits a JSON brief.
// Replace with a real chat.Model from lynx/models/{openai,anthropic,...}.
// ============================================================================

type stubModel struct {
	defaults *chat.Options
}

func newStubModel() *stubModel {
	opts, _ := chat.NewOptions("stub-model")
	return &stubModel{defaults: opts}
}

func (m *stubModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *stubModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

// Call walks the conversation and decides:
//
//   - first turn (only user message): emit a tool call so the
//     ToolMiddleware will execute the search tool;
//   - second turn (tool result is in history): emit the final JSON
//     brief.
func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	if !hasToolMessage(req.Messages) {
		return responseWithToolCall(`{"query":"agent frameworks 2026"}`), nil
	}
	return responseWithText(`{"summary":"Agent frameworks in 2026 are converging on GOAP planning, OODA tick loops, and unified tool models.","sources":["https://example.com/agents-2026"]}`), nil
}

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func hasToolMessage(messages []chat.Message) bool {
	for _, msg := range messages {
		if msg.Type() == chat.MessageTypeTool {
			return true
		}
	}
	return false
}

func responseWithText(text string) *chat.Response {
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp
}

func responseWithToolCall(args string) *chat.Response {
	calls := []*chat.ToolCallPart{
		{ID: "call_1", Name: "research_search", Arguments: args},
	}
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(calls),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{},
	)
	return resp
}

// ============================================================================
// Tool group — exposes a single "research_search" tool the LLM can call.
// ============================================================================

type researchToolGroup struct {
	tools []chat.Tool
}

func newResearchToolGroup() *researchToolGroup {
	tool, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "research_search",
			Description: "search the public web for sources on a topic",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			// Stub: pretend to search and return canned sources.
			return `[{"url":"https://example.com/agents-2026","title":"Agents in 2026"}]`, nil
		},
	)
	if err != nil {
		panic(err)
	}
	return &researchToolGroup{tools: []chat.Tool{tool}}
}

func (g *researchToolGroup) Metadata() core.ToolGroupMetadata {
	return core.SimpleToolGroupMetadata{RoleText: "research"}
}

func (g *researchToolGroup) Tools(_ context.Context) ([]core.AgentTool, error) {
	return g.tools, nil
}

// ============================================================================
// Misc helpers
// ============================================================================

// extractJSON pulls the first balanced JSON object out of text — the
// stub LLM is well-behaved, but real models often wrap their JSON in
// prose so this is the kind of cleanup a real example would need.
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return text
	}
	return text[start : end+1]
}

// eventLogger prints each event one-liner — illustrative only.
type eventLogger struct{}

func (eventLogger) Name() string { return "event-logger" }
func (eventLogger) OnEvent(e event.Event) {
	fmt.Printf("event: %-26s %s\n", e.EventName(), e.ProcessID())
}
