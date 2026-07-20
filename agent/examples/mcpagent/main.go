package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

// Domain types — the agent takes a Topic and produces a Brief.
type (
	Topic struct{ Title string }
	Brief struct {
		Topic   string
		Sources []string
	}
)

func main() {
	ctx := context.Background()

	chatClient, err := chatclient.New(newStubModel())
	if err != nil {
		log.Fatal(err)
	}

	srvT, cliT := sdkmcp.NewInMemoryTransports()
	srv := buildMCPServer()
	srvSession, err := srv.Connect(ctx, srvT, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer srvSession.Close()

	samplingHandler, err := lynxmcp.NewSamplingHandler(chatClient)
	if err != nil {
		log.Fatal(err)
	}
	cli := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "lynx-mcp-agent", Version: "v0.1.0"},
		&sdkmcp.ClientOptions{
			// Sampling: lets the MCP server "borrow" the engine LLM via
			// createMessage. This particular example doesn't exercise it,
			// but the wiring is part of a complete client.
			CreateMessageHandler: samplingHandler,
		},
	)
	cliSession, err := cli.Connect(ctx, cliT, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer cliSession.Close()

	toolSource := func(ctx context.Context) ([]tools.Tool, error) {
		return lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "research", Session: cliSession}}, lynxmcp.ToolOptions{
			MetaFunc: lynxmcp.MetaFromContext,
		})
	}

	a := agent.New(agent.AgentConfig{Name: "BriefingAgent", Description: "ask the LLM for a topic brief, with a remote MCP search tool", Actions: []agent.Action{agent.NewAction("brief", func(ctx context.Context, pc *agent.ProcessContext, in Topic) (Brief, error) {
		result, err := cliSession.GetPrompt(ctx, &sdkmcp.GetPromptParams{Name: "researcher_role", Arguments: map[string]string{"topic": in.Title}})
		if err != nil {
			return Brief{}, fmt.Errorf("get prompt: %w", err)
		}
		systemMessages := lynxmcp.PromptMessagesToChat(result.Messages)
		var systemPrompt strings.Builder
		for index := range systemMessages {
			systemPrompt.WriteString(systemMessages[index].Text())
		}
		ctx = lynxmcp.WithMeta(ctx, sdkmcp.Meta{"lynx.process_id": pc.Process().ID(), "lynx.action": "brief"})
		prompt := fmt.Sprintf("Use research_search to gather sources on %q, then reply with JSON: "+`{"sources":["..."]}`, in.Title)
		text, err := pc.Prompt(ctx, prompt, core.PromptConfig{System: systemPrompt.String()})
		if err != nil {
			return Brief{}, err
		}
		var parsed struct {
			Sources []string `json:"sources"`
		}
		_ = json.Unmarshal([]byte(text), &parsed)
		return Brief{Topic: in.Title, Sources: parsed.Sources}, nil
	}, agent.ActionConfig{ToolGroups: []core.ToolGroupRequirement{core.RequireToolGroup("research")}})}, Goals: []*agent.Goal{agent.NewOutputGoal[Brief](agent.GoalConfig{Description: "topic brief produced"})}})

	resolver, err := core.NewLazyToolGroupResolver(
		"mcp-research",
		core.ToolGroupInfo{Role: "research"},
		toolSource,
	)
	if err != nil {
		log.Fatal(err)
	}
	engine := agent.MustNewEngine(agent.EngineConfig{
		Chat:       agent.ChatCapability{Model: chatClient, Streamer: chatClient},
		Extensions: []agent.Extension{resolver},
	})
	if _, err := engine.Deploy(a); err != nil {
		log.Fatal(err)
	}

	process, err := engine.Run(
		ctx, a,
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
}

// ============================================================================
// In-memory MCP server: one tool + one prompt + meta-aware logging.
// ============================================================================

func buildMCPServer() *sdkmcp.Server {
	srv := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "research-server", Version: "v0.1.0"},
		nil,
	)

	// Tool — logs the _meta forwarded by the client to demonstrate
	// request-level metadata flow.
	srv.AddTool(
		&sdkmcp.Tool{
			Name:        "search",
			Description: "search the public web for sources on a topic",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		},
		func(_ context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			fmt.Printf("[mcp-server] tool call meta=%v\n", request.Params.Meta)
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{
					Text: `[{"url":"https://example.com/agents-2026","title":"Agents in 2026"}]`,
				}},
			}, nil
		},
	)

	// Prompt — returns a system message templated on the {topic}
	// argument the action passed.
	srv.AddPrompt(
		&sdkmcp.Prompt{
			Name:        "researcher_role",
			Description: "system prompt for a research analyst",
			Arguments: []*sdkmcp.PromptArgument{
				{Name: "topic", Required: true},
			},
		},
		func(_ context.Context, request *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			topic := request.Params.Arguments["topic"]
			return &sdkmcp.GetPromptResult{
				Messages: []*sdkmcp.PromptMessage{{
					Role: "assistant",
					Content: &sdkmcp.TextContent{
						Text: fmt.Sprintf(
							"You are a research analyst focused on %q. Cite sources you used.",
							topic,
						),
					},
				}},
			}, nil
		},
	)

	return srv
}

// ============================================================================
// Stub LLM — pretends to use the search tool, then emits JSON sources.
// ============================================================================

type stubModel struct{}

func newStubModel() *stubModel { return &stubModel{} }

func (m *stubModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if !hasToolMessage(request.Messages) {
		// First turn — emit a tool call. The MCP-backed tool name is
		// "research_search" because DefaultNaming prefixes the source
		// name to the descriptor name ("research" + "_" + "search").
		return responseWithToolCall("research_search", `{"query":"agent frameworks 2026"}`), nil
	}
	return responseWithText(`{"sources":["https://example.com/agents-2026"]}`), nil
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

func responseWithToolCall(name, args string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_1", Name: name, Arguments: args}))
	response, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
	return response
}
