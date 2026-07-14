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
	"github.com/Tangerg/lynx/agent/runtime"
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

	samplingHandler, err := lynxmcp.SamplingViaChatClient(chatClient)
	if err != nil {
		log.Fatal(err)
	}
	cli := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "lynx-mcp-agent", Version: "v0.1.0"},
		&sdkmcp.ClientOptions{
			// Sampling: lets the MCP server "borrow" the platform LLM via
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

	a := agent.New("BriefingAgent").
		Description("ask the LLM for a topic brief, with a remote MCP search tool").
		Actions(agent.NewAction("brief",
			func(ctx context.Context, pc *core.ProcessContext, in Topic) (Brief, error) {
				// 1. Fetch the system prompt from the MCP server.
				result, err := cliSession.GetPrompt(ctx, &sdkmcp.GetPromptParams{
					Name: "researcher_role",
					Arguments: map[string]string{
						"topic": in.Title,
					},
				})
				if err != nil {
					return Brief{}, fmt.Errorf("get prompt: %w", err)
				}
				systemMessages := lynxmcp.PromptMessagesToChat(result.Messages)
				var systemPrompt strings.Builder
				for index := range systemMessages {
					systemPrompt.WriteString(systemMessages[index].Text())
				}

				// 2. Attach process metadata to ctx — the MCP server's
				// tool handler reads it via req.Params.Meta.
				ctx = lynxmcp.WithMeta(ctx, sdkmcp.Meta{
					"lynx.process_id": pc.Process.ID(),
					"lynx.action":     "brief",
				})

				prompt := fmt.Sprintf(
					"Use research_search to gather sources on %q, then reply with JSON: "+
						`{"sources":["..."]}`,
					in.Title,
				)

				text, err := pc.PromptRunner().
					WithSystem(systemPrompt.String()).
					Generate(ctx, prompt)
				if err != nil {
					return Brief{}, err
				}

				var parsed struct {
					Sources []string `json:"sources"`
				}
				_ = json.Unmarshal([]byte(text), &parsed)
				return Brief{Topic: in.Title, Sources: parsed.Sources}, nil
			},
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor("research"),
			},
		)).
		Goals(agent.GoalProducing[Brief](core.Goal{Description: "topic brief produced"})).
		Build()

	resolver, err := runtime.NewMCPResolver("research", toolSource)
	if err != nil {
		log.Fatal(err)
	}
	platform := agent.NewPlatform(runtime.PlatformConfig{
		ChatClient: chatClient,
		Extensions: []core.Extension{resolver},
	})
	if err := platform.Deploy(a); err != nil {
		log.Fatal(err)
	}

	proc, err := platform.RunAgent(
		ctx, a,
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
		func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			fmt.Printf("[mcp-server] tool call meta=%v\n", req.Params.Meta)
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
		func(_ context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			topic := req.Params.Arguments["topic"]
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

func (m *stubModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	if !hasToolMessage(req.Messages) {
		// First turn — emit a tool call. The MCP-backed tool name is
		// "research_search" because DefaultNaming prefixes the source
		// name to the descriptor name ("research" + "_" + "search").
		return responseWithToolCall("research_search", `{"query":"agent frameworks 2026"}`), nil
	}
	return responseWithText(`{"sources":["https://example.com/agents-2026"]}`), nil
}

func (m *stubModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
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
	resp, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	return resp
}

func responseWithToolCall(name, args string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_1", Name: name, Arguments: args}))
	resp, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
	return resp
}
