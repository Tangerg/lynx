package mcp

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// SamplingHandler is the function shape MCP clients install on
// [sdkmcp.ClientOptions.CreateMessageHandler]. Defining the alias
// keeps user code from re-stating the unwieldy SDK signature.
type SamplingHandler = func(context.Context, *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error)

// SamplingViaChatClient builds a [SamplingHandler] that delegates the
// server's createMessage request to client. An MCP server can then
// "borrow" the client's LLM without ever owning credentials or model
// factories of its own.
//
// Translation:
//
//   - SystemPrompt → chat.WithSystemPrompt
//   - Messages     → chat.WithMessages (non-text content dropped)
//   - chat reply   → CreateMessageResult (Role=assistant, single
//     TextContent; StopReason mapped from chat.FinishReason).
//
// MaxTokens / Temperature / StopSequences / ModelPreferences are not
// forwarded: per the MCP spec these are hints the client may ignore,
// and the adapter otherwise uses the chatclient.Client's configured defaults.
//
// Concurrency is not bounded — wrap the returned handler with your own
// semaphore if your model quota requires it. Returns an error when
// client is nil — caller decides whether to surface or panic.
func SamplingViaChatClient(client *chatclient.Client) (SamplingHandler, error) {
	if client == nil {
		return nil, errors.New("mcp.SamplingViaChatClient: chatclient.Client must not be nil")
	}
	return func(ctx context.Context, req *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error) {
		if req == nil || req.Params == nil {
			return nil, errors.New("mcp.SamplingViaChatClient: sampling request params must not be nil")
		}

		messages := samplingMessagesToChat(req.Params.Messages)
		if req.Params.SystemPrompt != "" {
			messages = append([]chat.Message{chat.NewSystemMessage(req.Params.SystemPrompt)}, messages...)
		}
		chatReq := &chat.Request{Messages: messages}
		if req.Params.MaxTokens > 0 {
			value := req.Params.MaxTokens
			chatReq.Options.MaxTokens = &value
		}
		if req.Params.Temperature != 0 {
			value := req.Params.Temperature
			chatReq.Options.Temperature = &value
		}
		chatReq.Options.Stop = append([]string(nil), req.Params.StopSequences...)

		resp, err := client.Call(ctx, chatReq)
		if err != nil {
			return nil, fmt.Errorf("mcp.SamplingViaChatClient: sample via chat: %w", err)
		}
		return chatResponseToSamplingResult(resp), nil
	}, nil
}

func samplingMessagesToChat(messages []*sdkmcp.SamplingMessage) []chat.Message {
	out := make([]chat.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if converted, ok := chatMessageFromContent(msg.Role, msg.Content); ok {
			out = append(out, converted)
		}
	}
	return out
}

func chatResponseToSamplingResult(resp *chat.Response) *sdkmcp.CreateMessageResult {
	text := resp.Text()
	stop := "end_turn"
	if resp != nil && resp.First() != nil {
		stop = mapStopReason(resp.First().FinishReason)
	}
	return &sdkmcp.CreateMessageResult{
		Role:       "assistant",
		Content:    &sdkmcp.TextContent{Text: text},
		StopReason: stop,
	}
}

func mapStopReason(r chat.FinishReason) string {
	switch r {
	case chat.FinishReasonStop:
		return "end_turn"
	case chat.FinishReasonLength:
		return "max_tokens"
	case chat.FinishReasonToolCalls:
		return "tool_use"
	default:
		return string(r)
	}
}
