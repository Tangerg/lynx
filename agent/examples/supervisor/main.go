// supervisor demonstrates the "LLM-orchestrated multi-agent" pattern:
// a parent agent's LLM picks among sub-agents (each wrapped via
// runtime.AsChatTool) and chains them through chat.NewToolMiddleware.
//
// The parent's action body asks the LLM to brief a topic. The stub
// LLM:
//
//  1. first calls the "research-agent" sub-tool with {Title: ...} →
//     gets {Sources: [...]}
//  2. then calls the "summarize-agent" sub-tool with {Sources: [...]} →
//     gets {Summary: "..."}
//  3. finally emits the JSON Brief.
//
// chat.ToolMiddleware drives the LLM-tool loop; runtime.AsChatTool
// runs each sub-agent synchronously inside the parent process.
// Budget aggregation is automatic — the parent's Usage() sums the
// whole delegation tree.
//
// Run from repo root:
//
//	go run ./agent/examples/supervisor
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
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
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
	chatClient, err := chat.NewClientWithModel(newStubModel())
	if err != nil {
		log.Fatal(err)
	}

	platform := agent.NewPlatform(runtime.PlatformConfig{ChatClient: chatClient})

	// ---- sub-agents ---------------------------------------------------
	research := agent.New("research-agent").
		Description("find sources for a topic").
		Actions(agent.NewAction("search",
			func(_ context.Context, _ *core.ProcessContext, in Topic) (Sources, error) {
				return Sources{URLs: []string{"https://example.com/" + slug(in.Title)}}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[Sources](core.Goal{Description: "sources produced"})).
		Build()

	summarize := agent.New("summarize-agent").
		Description("summarize a list of sources").
		Actions(agent.NewAction("summarize",
			func(_ context.Context, _ *core.ProcessContext, in Sources) (Summary, error) {
				return Summary{
					Text: fmt.Sprintf("Synthesized findings from %d sources: %s", len(in.URLs), strings.Join(in.URLs, ", ")),
				}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[Summary](core.Goal{Description: "summary produced"})).
		Build()

	if err := platform.Deploy(research); err != nil {
		log.Fatal(err)
	}
	if err := platform.Deploy(summarize); err != nil {
		log.Fatal(err)
	}

	// ---- parent agent (the supervisor) -------------------------------
	parent := agent.New("supervisor").
		Description("orchestrates research + summarize via the LLM").
		Actions(agent.NewAction("brief",
			func(ctx context.Context, pc *core.ProcessContext, in Topic) (Brief, error) {
				researchTool := runtime.AsChatTool[Topic, Sources](platform, "research-agent")
				summarizeTool := runtime.AsChatTool[Sources, Summary](platform, "summarize-agent")

				callMW, streamMW := chat.NewToolMiddleware()
				req := pc.Chat().
					WithMiddlewares(callMW, streamMW).
					WithTools(researchTool, summarizeTool)

				prompt := fmt.Sprintf(
					"Brief me on %q. Use research-agent first to gather sources, "+
						"then summarize-agent to synthesise. Reply with JSON: "+
						`{"sources":[...],"summary":"..."}`,
					in.Title,
				)
				text, _, err := req.
					WithSystemPrompt("You are a supervisor that delegates to specialised agents.").
					WithUserPrompt(prompt).
					Call().
					Text(ctx)
				if err != nil {
					return Brief{}, err
				}

				var parsed struct {
					Sources []string `json:"sources"`
					Summary string   `json:"summary"`
				}
				if jsonErr := json.Unmarshal([]byte(extractJSON(text)), &parsed); jsonErr != nil {
					// LLM didn't follow the JSON cue — fall back to plaintext.
					parsed.Summary = strings.TrimSpace(text)
				}
				return Brief{
					Topic:   in.Title,
					Sources: parsed.Sources,
					Text:    parsed.Summary,
				}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[Brief](core.Goal{Description: "brief produced"})).
		Build()

	if err := platform.Deploy(parent); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(), parent,
		map[string]any{core.DefaultBindingName: Topic{Title: "agent frameworks in 2026"}},
		core.ProcessOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	brief, ok := core.ResultOfType[Brief](proc)
	if !ok {
		log.Fatalf("no Brief produced; status=%s; failure=%v", proc.Status(), proc.Failure())
	}

	cost, tokens, actions := proc.Usage()

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

type stubModel struct {
	defaults *chat.Options
}

func newStubModel() *stubModel {
	opts, _ := chat.NewOptions("stub-model")
	return &stubModel{defaults: opts}
}

func (m *stubModel) DefaultOptions() *chat.Options { return m.defaults }
func (m *stubModel) Info() chat.ModelInfo          { return chat.ModelInfo{Provider: "stub"} }

// Call walks the conversation:
//
//   - turn 1 (only user message) → call research-agent
//   - turn 2 (research result in history, no summary yet) → call summarize-agent
//   - turn 3 (summary in history) → emit final JSON
func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	toolHistory := collectToolReturns(req.Messages)
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

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

// collectToolReturns walks the conversation and returns name → result
// for every tool that has emitted a result.
func collectToolReturns(messages []chat.Message) map[string]string {
	out := map[string]string{}
	for _, msg := range messages {
		if msg.Type() != chat.MessageTypeTool {
			continue
		}
		if tm, ok := msg.(*chat.ToolMessage); ok {
			for _, r := range tm.ToolReturns {
				out[r.Name] = r.Result
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
	resp, _ := chat.NewResponse(
		[]*chat.Result{{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		}},
		&chat.ResponseMetadata{},
	)
	return resp
}

func responseWithToolCall(name, args string) *chat.Response {
	calls := []*chat.ToolCall{{ID: "call_" + name, Name: name, Arguments: args}}
	resp, _ := chat.NewResponse(
		[]*chat.Result{{
			AssistantMessage: chat.NewAssistantMessage(calls),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		}},
		&chat.ResponseMetadata{},
	)
	return resp
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
