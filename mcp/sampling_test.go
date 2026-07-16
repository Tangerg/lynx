package mcp_test

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

func TestNewSamplingHandlerRequiresClient(t *testing.T) {
	if _, err := lynxmcp.NewSamplingHandler(nil); err == nil {
		t.Fatal("NewSamplingHandler succeeded with a nil client")
	}
}

func TestNewSamplingHandlerForwardsRequestOptions(t *testing.T) {
	var captured *chat.Request
	model := chat.ModelFunc(func(_ context.Context, req *chat.Request) (*chat.Response, error) {
		captured = req
		message := chat.NewAssistantMessage(chat.NewTextPart("done"))
		return &chat.Response{Choices: []chat.Choice{{
			Message:      &message,
			FinishReason: chat.FinishReasonStop,
		}}}, nil
	})
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := lynxmcp.NewSamplingHandler(client)
	if err != nil {
		t.Fatal(err)
	}

	result, err := handler(t.Context(), &sdkmcp.CreateMessageRequest{
		Params: &sdkmcp.CreateMessageParams{
			SystemPrompt: "system",
			Messages: []*sdkmcp.SamplingMessage{{
				Role:    "user",
				Content: &sdkmcp.TextContent{Text: "hello"},
			}},
			MaxTokens:     128,
			Temperature:   0.25,
			StopSequences: []string{"STOP"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "end_turn" {
		t.Fatalf("StopReason = %q, want end_turn", result.StopReason)
	}
	if captured == nil {
		t.Fatal("model did not receive a request")
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Role != chat.RoleSystem || captured.Messages[1].Text() != "hello" {
		t.Fatalf("Messages = %#v", captured.Messages)
	}
	if captured.Options.MaxTokens == nil || *captured.Options.MaxTokens != 128 {
		t.Fatalf("MaxTokens = %v, want 128", captured.Options.MaxTokens)
	}
	if captured.Options.Temperature == nil || *captured.Options.Temperature != 0.25 {
		t.Fatalf("Temperature = %v, want 0.25", captured.Options.Temperature)
	}
	if len(captured.Options.Stop) != 1 || captured.Options.Stop[0] != "STOP" {
		t.Fatalf("Stop = %v, want [STOP]", captured.Options.Stop)
	}
}
