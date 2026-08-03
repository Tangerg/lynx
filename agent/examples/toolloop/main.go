package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type addInput struct {
	A int `json:"a"`
	B int `json:"b"`
}

type addTool struct{}

func (addTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "add",
		Description: "add two integers",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"],"additionalProperties":false}`),
	}
}

func (addTool) Call(_ context.Context, arguments string) (string, error) {
	var input addInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("decode add input: %w", err)
	}
	return fmt.Sprint(input.A + input.B), nil
}

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output io.Writer) error {
	registry, err := tools.NewRegistry(addTool{})
	if err != nil {
		return err
	}

	round := 0
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		round++
		if round == 1 {
			return response(chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID:        "call-1",
				Name:      "add",
				Arguments: `{"a":2,"b":3}`,
			})), chat.FinishReasonToolCalls), nil
		}
		last := request.Messages[len(request.Messages)-1]
		result := last.Parts[0].ToolResult
		return response(chat.NewAssistantMessage(chat.NewTextPart(result.Result)), chat.FinishReasonStop), nil
	})

	runner, err := toolloop.NewRunner(model, toolloop.Config{})
	if err != nil {
		return err
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("What is 2 + 3?")))
	if err != nil {
		return err
	}
	request.Tools = registry.Definitions()

	for event, eventErr := range runner.Run(ctx, request, registry) {
		if eventErr != nil {
			return eventErr
		}
		switch {
		case event.Kind == toolloop.EventToolResult:
			if _, err := fmt.Fprintf(output, "tool %s => %s\n", event.ToolResult.Name, event.ToolResult.Result); err != nil {
				return err
			}
		case event.Kind == toolloop.EventModelResponse && event.Final:
			if _, err := fmt.Fprintf(output, "assistant => %s\n", event.Response.Text()); err != nil {
				return err
			}
		}
	}
	return nil
}

func response(message chat.Message, reason chat.FinishReason) *chat.Response {
	return &chat.Response{Choices: []chat.Choice{{
		Index:        0,
		Message:      &message,
		FinishReason: reason,
	}}}
}
