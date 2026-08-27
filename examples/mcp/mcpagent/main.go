package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
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

	model := &stubModel{}

	srvT, cliT := sdkmcp.NewInMemoryTransports()
	srv := buildMCPServer()
	srvSession, err := srv.Connect(ctx, srvT, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer srvSession.Close()

	cli := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "scope-mcp-agent", Version: "v0.1.0"},
		nil,
	)
	cliSession, err := cli.Connect(ctx, cliT, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer cliSession.Close()

	loadTools := func(ctx context.Context) ([]tool.Tool, error) {
		return scopemcp.Tools(ctx, []scopemcp.ToolSource{{Name: "research", Session: cliSession}}, scopemcp.ToolsConfig{
			MetaFunc: scopemcp.MetaFromContext,
		})
	}
	topic := Topic{Title: "agent frameworks in 2026"}
	promptResult, err := cliSession.GetPrompt(ctx, &sdkmcp.GetPromptParams{
		Name: "researcher_role", Arguments: map[string]string{"topic": topic.Title},
	})
	if err != nil {
		log.Fatal(fmt.Errorf("get prompt: %w", err))
	}
	systemMessages, err := scopemcp.PromptMessagesToChat(promptResult.Messages)
	if err != nil {
		log.Fatal(fmt.Errorf("convert MCP prompt messages: %w", err))
	}
	var systemPrompt strings.Builder
	for index := range systemMessages {
		systemPrompt.WriteString(systemMessages[index].Text())
	}
	availableTools, err := loadTools(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		log.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "example.mcp_briefing",
		Description:   "Ask the model for a topic brief using a remote MCP search tool.",
		Version:       "1.0.0",
		MaxModelCalls: 3,
	})
	if err != nil {
		log.Fatal(err)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Client: client,
		Tools:  availableTools,
	})
	if err != nil {
		log.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           definition,
		Dispatcher:           dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("example-mcp-briefing-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-mcp-briefing-configuration")),
	})
	if err != nil {
		log.Fatal(err)
	}
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	outputFormat, err := chat.NewOutputFormat(chat.OutputFormatJSON)
	if err != nil {
		log.Fatal(err)
	}
	prompt := fmt.Sprintf("Use research_search to gather source URLs on %q.", topic.Title)
	input, err := agent.EncodeInput(interaction.Input{Messages: []chat.Message{
		chat.NewSystemMessage(systemPrompt.String()),
		chat.NewUserMessage(chat.NewTextPart(prompt)),
	}, Options: chat.Options{OutputFormat: &outputFormat}})
	if err != nil {
		log.Fatal(err)
	}
	ctx = scopemcp.WithMeta(ctx, sdkmcp.Meta{"scope.example": "mcp-agent"})
	result, err := engine.Run(ctx, deployment, input)
	if err != nil {
		log.Fatal(err)
	}
	erased, ok := result.Output()
	if !ok {
		log.Fatalf("no interaction output produced; status=%s", result.Status())
	}
	output, err := erased.Decode[interaction.Output]()
	if err != nil {
		log.Fatal(err)
	}
	if output.ModelResponse == nil {
		log.Fatalf("unexpected interaction completion source %q", output.Source)
	}
	var parsed struct {
		Sources []string `json:"sources"`
	}
	if err := json.Unmarshal([]byte(output.ModelResponse.Text()), &parsed); err != nil {
		log.Fatal(fmt.Errorf("decode brief: %w", err))
	}
	brief := Brief{Topic: topic.Title, Sources: parsed.Sources}

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

func (s *stubModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if !hasToolMessage(request.Messages) {
		// First turn — emit a tool call. The MCP-backed tool name is
		// "research_search" because DefaultNaming prefixes the source
		// name to the descriptor name ("research" + "_" + "search").
		return responseWithToolCall("research_search", `{"query":"agent frameworks 2026"}`), nil
	}
	return responseWithText(`{"sources":["https://example.com/agents-2026"]}`), nil
}

func (s *stubModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := s.Call(ctx, request)
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
	response, _ := chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	return response
}

func responseWithToolCall(name, args string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_1", Name: name, Arguments: args}))
	response, _ := chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	return response
}
