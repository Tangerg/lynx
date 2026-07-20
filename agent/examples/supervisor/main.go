package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// Domain types
type (
	Topic   struct{ Title string }
	Sources struct{ URLs []string }
	Summary struct{ Text string }
	Brief   struct {
		Topic   string
		Sources []string
		Text    string
	}
)

func main() {
	chatClient, err := chatclient.New(newStubModel())
	if err != nil {
		log.Fatal(err)
	}

	engine := agent.MustNewEngine(agent.EngineConfig{Chat: agent.ChatCapability{Model: chatClient, Streamer: chatClient}})

	// ---- sub-agents ---------------------------------------------------
	research := agent.New(agent.AgentConfig{Name: "research-agent", Description: "find sources for a topic", Actions: []agent.Action{agent.NewAction("search", func(_ context.Context, _ *agent.ProcessContext, in Topic) (Sources, error) {
		return Sources{URLs: []string{"https://example.com/" + slug(in.Title)}}, nil
	}, agent.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[Sources](agent.GoalConfig{Description: "sources produced"})}})

	summarize := agent.New(agent.AgentConfig{Name: "summarize-agent", Description: "summarize a list of sources", Actions: []agent.Action{agent.NewAction("summarize", func(_ context.Context, _ *agent.ProcessContext, in Sources) (Summary, error) {
		return Summary{Text: fmt.Sprintf("Synthesized findings from %d sources: %s", len(in.URLs), strings.Join(in.URLs, ", "))}, nil
	}, agent.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[Summary](agent.GoalConfig{Description: "summary produced"})}})

	if _, err := engine.Deploy(context.Background(), research); err != nil {
		log.Fatal(err)
	}
	if _, err := engine.Deploy(context.Background(), summarize); err != nil {
		log.Fatal(err)
	}

	// ---- parent agent (the supervisor) -------------------------------
	parent := agent.New(agent.AgentConfig{Name: "supervisor", Description: "orchestrates research + summarize via the LLM", Actions: []agent.Action{agent.NewAction("brief", func(ctx context.Context, pc *agent.ProcessContext, in Topic) (Brief, error) {
		researchTool, _ := runtime.NewAgentTool[Topic, Sources](engine, "research-agent")
		summarizeTool, _ := runtime.NewAgentTool[Sources, Summary](engine, "summarize-agent")
		prompt := fmt.Sprintf("Brief me on %q. Use research-agent first to gather sources, "+"then summarize-agent to synthesise. Reply with JSON: "+`{"sources":[...],"summary":"..."}`, in.Title)
		text, err := pc.Prompt(ctx, prompt, agent.PromptConfig{
			System: "You are a supervisor that delegates to specialised agents.",
			Tools:  []tools.Tool{researchTool, summarizeTool},
		})
		if err != nil {
			return Brief{}, err
		}
		var parsed struct {
			Sources []string `json:"sources"`
			Summary string   `json:"summary"`
		}
		if jsonErr := json.Unmarshal([]byte(extractJSON(text)), &parsed); jsonErr != nil {
			parsed.Summary = strings.TrimSpace(text)
		}
		return Brief{Topic: in.Title, Sources: parsed.Sources, Text: parsed.Summary}, nil
	}, agent.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[Brief](agent.GoalConfig{Description: "brief produced"})}})

	if _, err := engine.Deploy(context.Background(), parent); err != nil {
		log.Fatal(err)
	}

	process, err := engine.Run(
		context.Background(), parent,
		agent.Input(Topic{Title: "agent frameworks in 2026"}),
		agent.ProcessOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	brief, ok := agent.Result[Brief](process)
	if !ok {
		log.Fatalf("no Brief produced; status=%s; failure=%v", process.Status(), process.Failure())
	}

	cost, tokens, actions := process.Usage()

	fmt.Println("\n--- result ---")
	fmt.Printf("topic:   %s\n", brief.Topic)
	fmt.Printf("sources: %v\n", brief.Sources)
	fmt.Printf("summary: %s\n", brief.Text)
	fmt.Printf("\n--- usage (parent + every sub-agent) ---\n")
	fmt.Printf("cost: $%.4f  tokens: %d  actions: %d\n", cost, tokens, actions)
}

// ============================================================================
// Stub LLM — sequences research → summarize → final JSON.
// ============================================================================

type stubModel struct{}

func newStubModel() *stubModel { return &stubModel{} }

// Call walks the conversation:
//
//   - turn 1 (only user message) → call research-agent
//   - turn 2 (research result in history, no summary yet) → call summarize-agent
//   - turn 3 (summary in history) → emit final JSON
func (m *stubModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	toolHistory := collectToolReturns(request.Messages)
	switch {
	case !contains(toolHistory, "research-agent"):
		return responseWithToolCall("research-agent", `{"Title":"agent frameworks in 2026"}`), nil
	case !contains(toolHistory, "summarize-agent"):
		// pull URLs out of the research result so we feed them on
		var sources Sources
		_ = json.Unmarshal([]byte(toolHistory["research-agent"]), &sources)
		raw, _ := json.Marshal(sources)
		return responseWithToolCall("summarize-agent", string(raw)), nil
	default:
		var summary Summary
		_ = json.Unmarshal([]byte(toolHistory["summarize-agent"]), &summary)
		var sources Sources
		_ = json.Unmarshal([]byte(toolHistory["research-agent"]), &sources)
		raw, _ := json.Marshal(struct {
			Sources []string `json:"sources"`
			Summary string   `json:"summary"`
		}{Sources: sources.URLs, Summary: summary.Text})
		return responseWithText(string(raw)), nil
	}
}

func (m *stubModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := m.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

// collectToolReturns walks the conversation and returns name → result
// for every tool that has emitted a result.
func collectToolReturns(messages []chat.Message) map[string]string {
	out := map[string]string{}
	for _, msg := range messages {
		if msg.Role != chat.RoleTool {
			continue
		}
		for index := range msg.Parts {
			if result := msg.Parts[index].ToolResult; result != nil {
				out[result.Name] = result.Result
			}
		}
	}
	return out
}

func contains(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}

func responseWithText(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	response, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	return response
}

func responseWithToolCall(name, args string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_" + name, Name: name, Arguments: args}))
	response, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
	return response
}

// ============================================================================
// Helpers
// ============================================================================

func slug(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return text
	}
	return text[start : end+1]
}
