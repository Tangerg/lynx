package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
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
	BriefContent struct {
		Sources []string `json:"sources"`
		Summary string   `json:"summary"`
	}
)

const researchToolRole = "research"

func main() {
	chatClient, err := chatclient.New(newStubModel(), chatclient.Config{})
	if err != nil {
		log.Fatal(err)
	}

	resolver := researchToolResolver{group: newResearchToolGroup()}

	a := agent.New(agent.AgentConfig{Name: "BriefingAgent", Description: "ask the LLM for a topic brief, with a search tool available", Actions: []agent.Action{agent.NewAction("brief", func(ctx context.Context, pc *agent.ProcessContext, in Topic) (Brief, error) {
		prompt := fmt.Sprintf("Write a one-paragraph brief on %q. Use the `research_search` tool to gather sources first, then return the summary and source URLs.", in.Title)
		content, err := agent.Prompt(ctx, pc, prompt, agent.PromptConfig{
			System: "You are a research analyst. Cite sources you used.",
		}, chatclient.JSON[BriefContent]())
		if err != nil {
			return Brief{}, err
		}
		return Brief{Topic: in.Title, Sources: content.Sources, Summary: content.Summary}, nil
	}, agent.ActionConfig{ToolGroups: []string{researchToolRole}})}, Goals: []*agent.Goal{agent.NewOutputGoal[Brief](agent.GoalConfig{Description: "topic brief produced"})}})

	engine := agent.MustNewEngine(agent.EngineConfig{
		Chat:       agent.Chat(chatClient),
		Extensions: []agent.Extension{resolver, &eventLogger{}},
	})
	if _, err := engine.Deploy(context.Background(), a); err != nil {
		log.Fatal(err)
	}

	process, err := engine.Run(
		context.Background(), a,
		agent.Input(Topic{Title: "agent frameworks in 2026"}),
		agent.ProcessOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	brief, ok := agent.Result[Brief](process)
	if !ok {
		log.Fatalf("no Brief produced; status=%s", process.Status())
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

type stubModel struct{}

func newStubModel() *stubModel { return &stubModel{} }

// Call walks the conversation and decides:
//
//   - first turn (only user message): emit a tool call so the
//     tool loop will execute the search tool;
//   - second turn (tool result is in history): emit the final JSON
//     brief.
func (m *stubModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if !hasToolMessage(request.Messages) {
		return responseWithToolCall(`{"query":"agent frameworks 2026"}`), nil
	}
	return responseWithText(`{"summary":"Agent frameworks in 2026 are converging on GOAP planning, OODA tick loops, and unified tool models.","sources":["https://example.com/agents-2026"]}`), nil
}

func (m *stubModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := m.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func hasToolMessage(messages []chat.Message) bool {
	for _, msg := range messages {
		if msg.Role == chat.RoleTool {
			return true
		}
	}
	return false
}

func responseWithText(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	response, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	return response
}

func responseWithToolCall(args string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_1", Name: "research_search", Arguments: args}))
	response, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
	return response
}

// ============================================================================
// Tool group — exposes a single "research_search" tool the LLM can call.
// ============================================================================

type researchToolGroup struct {
	tools []tool.Tool
}

type researchToolResolver struct {
	group *researchToolGroup
}

func (researchToolResolver) Name() string { return "research-tools" }

func (r researchToolResolver) Resolve(_ context.Context, role string) (agent.ToolGroup, bool, error) {
	if role != researchToolRole {
		return nil, false, nil
	}
	return r.group, true, nil
}

type researchSearchInput struct {
	Query string `json:"query" jsonschema:"required"`
}

type researchSearchTool struct{}

func (researchSearchTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "research_search",
		Description: "search the public web for sources on a topic",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`),
	}
}

func (researchSearchTool) Call(_ context.Context, arguments string) (string, error) {
	var input researchSearchInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("decode research search input: %w", err)
	}
	// Stub: pretend to search and return canned sources.
	return `[{"url":"https://example.com/agents-2026","title":"Agents in 2026"}]`, nil
}

func newResearchToolGroup() *researchToolGroup {
	return &researchToolGroup{tools: []tool.Tool{researchSearchTool{}}}
}

func (g *researchToolGroup) Tools(_ context.Context) ([]tool.Tool, error) {
	return g.tools, nil
}

// eventLogger prints each event one-liner — illustrative only.
type eventLogger struct{}

func (eventLogger) Name() string { return "event-logger" }
func (eventLogger) OnEvent(_ context.Context, e event.Event) {
	fmt.Printf("event: %-26s %s\n", e.Kind(), e.ProcessID())
}
