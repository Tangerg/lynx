package mcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/chat"
)

// SamplingHandler is the function shape MCP clients install on
// [sdkmcp.ClientOptions.CreateMessageHandler]. Defining the alias
// keeps user code from re-stating the unwieldy SDK signature.
type SamplingHandler = func(context.Context, *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error)

// ChatCaller is the synchronous chat capability needed by MCP sampling. The
// interface is defined at this consumption boundary so callers may supply a
// composed chat client, a chat.Model, or a focused test implementation without
// coupling this package to a concrete client type.
type ChatCaller interface {
	Call(ctx context.Context, request *chat.Request) (*chat.Response, error)
}

// NewSamplingHandler returns a [SamplingHandler] that delegates the server's
// createMessage request to client. An MCP server can then "borrow" the
// client's LLM without owning credentials or model factories.
//
// Translation:
//
//   - SystemPrompt → a prepended system message
//   - Messages     → core chat messages (non-text content dropped)
//   - MaxTokens     → chat.Options.MaxTokens
//   - Temperature   → chat.Options.Temperature
//   - StopSequences → chat.Options.Stop
//   - chat reply   → CreateMessageResult (Role=assistant, single
//     TextContent; StopReason mapped from chat.FinishReason).
//
// ModelPreferences is not forwarded: model selection belongs to the supplied
// [ChatCaller], while MCP preferences are advisory.
//
// Concurrency is not bounded — wrap the returned handler with your own
// semaphore if your model quota requires it. Returns an error when
// caller is nil — caller decides whether to surface or panic.
func NewSamplingHandler(caller ChatCaller) (SamplingHandler, error) {
	if isNilChatCaller(caller) {
		return nil, ErrNilChatCaller
	}
	return func(ctx context.Context, req *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error) {
		if req == nil || req.Params == nil {
			return nil, errors.New("mcp.NewSamplingHandler: sampling request params must not be nil")
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

		resp, err := caller.Call(ctx, chatReq)
		if err != nil {
			return nil, fmt.Errorf("mcp.NewSamplingHandler: sample via chat: %w", err)
		}
		result, err := chatResponseToSamplingResult(resp)
		if err != nil {
			return nil, fmt.Errorf("mcp.NewSamplingHandler: convert chat response: %w", err)
		}
		return result, nil
	}, nil
}

func isNilChatCaller(caller ChatCaller) bool {
	value := reflect.ValueOf(caller)
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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

func chatResponseToSamplingResult(resp *chat.Response) (*sdkmcp.CreateMessageResult, error) {
	if err := resp.Validate(); err != nil {
		return nil, err
	}
	text := resp.Text()
	stop := "end_turn"
	if first := resp.First(); first != nil {
		stop = mapStopReason(first.FinishReason)
	}
	return &sdkmcp.CreateMessageResult{
		Role:       "assistant",
		Content:    &sdkmcp.TextContent{Text: text},
		StopReason: stop,
	}, nil
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
