package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
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
// and lynx defers to the chat.Client's configured defaults.
//
// Concurrency is not bounded — wrap the returned handler with your own
// semaphore if your model quota requires it. Panics if client is nil.
func SamplingViaChatClient(client *chat.Client) SamplingHandler {
	if client == nil {
		panic("mcp: nil chat.Client")
	}
	return func(ctx context.Context, req *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error) {
		if req == nil || req.Params == nil {
			return nil, fmt.Errorf("sampling request: params must not be nil")
		}

		chatReq := client.Chat().WithMessages(samplingMessagesToChat(req.Params.Messages)...)
		if req.Params.SystemPrompt != "" {
			chatReq = chatReq.WithSystemPrompt(req.Params.SystemPrompt)
		}

		resp, err := chatReq.Call().Response(ctx)
		if err != nil {
			return nil, fmt.Errorf("sample via chat: %w", err)
		}
		return chatResponseToSamplingResult(resp), nil
	}
}

func samplingMessagesToChat(messages []*sdkmcp.SamplingMessage) []chat.Message {
	out := make([]chat.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		text := textOfContent(msg.Content)
		if text == "" {
			continue
		}
		if string(msg.Role) == "assistant" {
			out = append(out, chat.NewAssistantMessage(text))
		} else {
			out = append(out, chat.NewUserMessage(text))
		}
	}
	return out
}

func chatResponseToSamplingResult(resp *chat.Response) *sdkmcp.CreateMessageResult {
	text, stop := "", "endTurn"
	if resp != nil {
		if r := resp.Result(); r != nil {
			if r.AssistantMessage != nil {
				text = r.AssistantMessage.Text
			}
			if r.Metadata != nil {
				stop = mapStopReason(r.Metadata.FinishReason)
			}
		}
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
		return "endTurn"
	case chat.FinishReasonLength:
		return "maxTokens"
	case chat.FinishReasonToolCalls:
		return "toolUse"
	default:
		return string(r)
	}
}
