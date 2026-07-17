package mcp_test

import (
	"context"
	"errors"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

type nilChatCaller struct{}

func (*nilChatCaller) Call(context.Context, *chat.Request) (*chat.Response, error) {
	panic("typed nil chat caller was used")
}

func TestNewSamplingHandlerRequiresCaller(t *testing.T) {
	var typedNil *nilChatCaller
	for _, caller := range []lynxmcp.ChatCaller{nil, typedNil} {
		if _, err := lynxmcp.NewSamplingHandler(caller); !errors.Is(err, lynxmcp.ErrNilChatCaller) {
			t.Fatalf("NewSamplingHandler error = %v, want ErrNilChatCaller", err)
		}
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
	handler, err := lynxmcp.NewSamplingHandler(model)
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

func TestSamplingHandlerRejectsInvalidChatResponse(t *testing.T) {
	user := chat.NewUserMessage(chat.NewTextPart("not an assistant response"))
	tests := []struct {
		name     string
		response *chat.Response
	}{
		{name: "nil response"},
		{
			name: "invalid response",
			response: &chat.Response{Choices: []chat.Choice{{
				Message: &user,
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return test.response, nil
			})
			handler, err := lynxmcp.NewSamplingHandler(caller)
			if err != nil {
				t.Fatal(err)
			}
			_, err = handler(t.Context(), &sdkmcp.CreateMessageRequest{Params: &sdkmcp.CreateMessageParams{}})
			if !errors.Is(err, chat.ErrInvalidResponse) {
				t.Fatalf("handler error = %v, want chat.ErrInvalidResponse", err)
			}
		})
	}
}
