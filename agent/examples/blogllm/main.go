package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
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

const researchToolRole = "research"

func main() {
	chatClient, err := chatclient.New(newStubModel())
	if err != nil {
		log.Fatal(err)
	}

	resolver := researchToolResolver{group: newResearchToolGroup()}

	a := agent.New(agent.AgentConfig{Name: "BriefingAgent", Description: "ask the LLM for a topic brief, with a search tool available", Actions: []agent.Action{agent.NewAction("brief", func(ctx context.Context, pc *agent.ProcessContext, in Topic) (Brief, error) {
		prompt := fmt.Sprintf("Write a one-paragraph brief on %q. "+"Use the `research_search` tool to gather sources first; "+"then summarise. Reply with JSON: "+`{"summary":"...","sources":["..."]}`, in.Title)
		text, err := pc.Prompt(ctx, prompt, agent.PromptConfig{
			System: "You are a research analyst. Cite sources you used.",
		})
		if err != nil {
			return Brief{}, err
		}
		var parsed struct {
			Summary string   `json:"summary"`
			Sources []string `json:"sources"`
		}
		if jsonErr := json.Unmarshal([]byte(extractJSON(text)), &parsed); jsonErr != nil {
			parsed.Summary = strings.TrimSpace(text)
		}
		return Brief{Topic: in.Title, Sources: parsed.Sources, Summary: parsed.Summary}, nil
	}, agent.ActionConfig{ToolGroups: []agent.ToolGroupRequirement{agent.RequireToolGroup(researchToolRole)}})}, Goals: []*agent.Goal{agent.NewOutputGoal[Brief](agent.GoalConfig{Description: "topic brief produced"})}})

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
	tools []tools.Tool
}

type researchToolResolver struct {
	group *researchToolGroup
}

func (researchToolResolver) Name() string { return "research-tools" }

func (r researchToolResolver) Resolve(_ context.Context, requirement agent.ToolGroupRequirement) (agent.ToolGroup, bool, error) {
	if requirement.Role != researchToolRole {
		return nil, false, nil
	}
	return r.group, true, nil
}

type researchSearchInput struct {
	Query string `json:"query" jsonschema:"required"`
}

func newResearchToolGroup() *researchToolGroup {
	tool, err := tools.New[researchSearchInput, string](
		tools.Config{
			Name:        "research_search",
			Description: "search the public web for sources on a topic",
		},
		func(context.Context, researchSearchInput) (string, error) {
			// Stub: pretend to search and return canned sources.
			return `[{"url":"https://example.com/agents-2026","title":"Agents in 2026"}]`, nil
		},
	)
	if err != nil {
		panic(err)
	}
	return &researchToolGroup{tools: []tools.Tool{tool}}
}

func (g *researchToolGroup) Info() agent.ToolGroupInfo {
	return agent.ToolGroupInfo{Role: researchToolRole}
}

func (g *researchToolGroup) Tools(_ context.Context) ([]tools.Tool, error) {
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
func (eventLogger) OnEvent(_ context.Context, e event.Event) {
	fmt.Printf("event: %-26s %s\n", e.Kind(), e.ProcessID())
}
