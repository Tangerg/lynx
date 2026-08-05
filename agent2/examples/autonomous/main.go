// Command autonomous demonstrates an Interaction in which the model chooses a
// Tool from environment feedback and decides when to stop.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	client, err := chatclient.New(&calculatorModel{}, chatclient.Config{})
	if err != nil {
		return err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "example.autonomous_calculator",
		Description:   "Use available Tools until the requested calculation is complete.",
		Version:       "1.0.0",
		MaxModelCalls: 3,
	})
	if err != nil {
		return err
	}
	dispatcher, err := interaction.NewDispatcher(interaction.DispatcherConfig{
		Client: client,
		Tools:  []tool.Tool{additionTool{}},
	})
	if err != nil {
		return err
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           definition,
		Dispatcher:           dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("example-autonomous-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("example-autonomous-configuration")),
	})
	if err != nil {
		return err
	}
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		return err
	}
	defer engine.Close()
	input, err := agent.EncodeInput(interaction.Input{Messages: []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("What is 20 + 22?")),
	}})
	if err != nil {
		return err
	}
	result, err := engine.Run(ctx, deployment, input)
	if err != nil {
		return err
	}
	erased, ok := result.Output()
	if !ok {
		return fmt.Errorf("autonomous Process ended with %s", result.Status())
	}
	decoded, err := agent.DecodeOutput[interaction.Output](erased)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, decoded.ModelResponse.Text())
	return err
}

type additionTool struct{}

func (additionTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "add",
		Description: "Add two numbers and return their sum.",
		InputSchema: json.RawMessage(`{
            "type":"object",
            "properties":{"left":{"type":"number"},"right":{"type":"number"}},
            "required":["left","right"],
            "additionalProperties":false
        }`),
	}
}

func (additionTool) Call(_ context.Context, arguments string) (string, error) {
	var input struct {
		Left  float64 `json:"left"`
		Right float64 `json:"right"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", err
	}
	return fmt.Sprintf("%g", input.Left+input.Right), nil
}

type calculatorModel struct{ calls int }

func (model *calculatorModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.calls++
	switch model.calls {
	case 1:
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID: "calculation_1", Name: "add", Arguments: `{"left":20,"right":22}`,
		}))
		return &chat.Response{Choices: []chat.Choice{{
			Index: 0, Message: &message, FinishReason: "tool_calls",
		}}}, nil
	case 2:
		last := request.Messages[len(request.Messages)-1]
		if last.Role != chat.RoleTool || len(last.Parts) != 1 ||
			last.Parts[0].ToolResult == nil || last.Parts[0].ToolResult.Result != "42" {
			return nil, errors.New("model did not receive the addition result")
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("20 + 22 = 42"))
		return &chat.Response{Choices: []chat.Choice{{
			Index: 0, Message: &message, FinishReason: "stop",
		}}}, nil
	default:
		return nil, errors.New("interaction did not stop after receiving the Tool result")
	}
}
