package function

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/function"
)

// Support is a generic structure designed for managing and executing registered functions
// within a chat-based system. It provides thread-safe access to a registry of functions
// and serves as a foundational component for systems requiring extensible function handling.
//
// Type Parameters:
//   - O: Represents the options for the chat prompt, typically used to configure or customize
//     chat requests. This type should conform to request.ChatRequestOptions.
//   - M: Represents the metadata for chat responses, providing additional context or information
//     about the generation process. This type should conform to result.ChatResultMetadata.
//
// Fields:
//   - mu: A sync.RWMutex that ensures thread-safe access to the `register` field. This
//     allows concurrent reads while preventing data races during modifications.
//   - register: A map that holds registered functions, keyed by their unique names. The
//     values are instances of function.Function, enabling dynamic function execution.
//
// Usage:
// The Support struct is designed to be extended and not directly instantiated. Custom
// implementations can embed Support to build on its functionality, such as managing
// dynamic function registrations in a chat system.
type Support[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	mu       sync.RWMutex
	register map[string]function.Function
}

func (s *Support[O, M]) Functions() map[string]function.Function {
	return s.register
}

func (s *Support[O, M]) MerageOptionsAndFunctions(options function.Options, funcs ...function.Function) []function.Function {
	rv := make([]function.Function, 0)
	rv = append(rv, funcs...)
	rv = append(rv, options.Functions()...)
	options.SetFunctions([]function.Function{})
	return rv
}

func (s *Support[O, M]) RegisterFunctions(funcs ...function.Function) {
	if len(funcs) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.register == nil {
		s.register = map[string]function.Function{}
	}
	for _, f := range funcs {
		s.register[f.Name()] = f
	}
}

func (s *Support[O, M]) FindFunctions(names ...string) []function.Function {
	names = lo.Uniq(names)

	s.mu.RLock()
	defer s.mu.RUnlock()

	rv := make([]function.Function, 0, len(names))

	for _, name := range names {
		f, ok := s.register[name]
		if ok {
			rv = append(rv, f)
		}
	}

	return rv
}

func (s *Support[O, M]) HandleToolCalls(ctx context.Context, req *request.ChatRequest[O], res *response.ChatResponse[M]) ([]message.ChatMessage, error) {
	var toolcallResult model.Result[*message.AssistantMessage, M]
	for _, r := range res.Results() {
		if r.Output().HasToolCalls() {
			toolcallResult = r
			break
		}
	}
	if toolcallResult == nil {
		return nil, errors.New("no tool call result found in the response")
	}
	toolMessage, err := s.executeFunctions(ctx, toolcallResult.Output())
	if err != nil {
		return nil, err
	}
	return s.buildToolCallConversation(
		req.Instructions(),
		toolcallResult.Output(),
		toolMessage,
	), nil
}

func (s *Support[O, M]) buildToolCallConversation(msgs []message.ChatMessage, assistantMessage *message.AssistantMessage, toolMessage *message.ToolCallsMessage) []message.ChatMessage {
	rv := make([]message.ChatMessage, 0, len(msgs)+2)
	copy(rv, msgs)
	rv = append(rv, assistantMessage, toolMessage)
	return rv
}

func (s *Support[O, M]) executeFunctions(ctx context.Context, assistantMessage *message.AssistantMessage) (*message.ToolCallsMessage, error) {
	resps := make([]*message.ToolCallResponse, 0, len(assistantMessage.ToolCalls()))
	for _, toolCall := range assistantMessage.ToolCalls() {
		f, ok := s.register[toolCall.Name]
		if !ok {
			return nil, fmt.Errorf("no function callback found for function name: %s", toolCall.Name)
		}
		resp, err := f.Call(ctx, toolCall.Arguments)
		if err != nil {
			return nil, err
		}
		resps = append(resps, &message.ToolCallResponse{
			ID:   toolCall.ID,
			Name: toolCall.Name,
			Data: resp,
		})
	}
	return message.NewToolCallsMessage(resps, nil), nil
}

func (s *Support[O, M]) IsToolCallChatResponse(res *response.ChatResponse[M], finishReasons []result.FinishReason) bool {
	for _, r := range res.Results() {
		if s.isToolCallChatResult(r, finishReasons) {
			return true
		}
	}
	return false
}

func (s *Support[O, M]) isToolCallChatResult(assistantMessage model.Result[*message.AssistantMessage, M], finishReasons []result.FinishReason) bool {
	if !assistantMessage.Output().HasToolCalls() {
		return false
	}
	reason := assistantMessage.Metadata().FinishReason()
	for _, finishReason := range finishReasons {
		if strings.ToLower(reason.String()) == strings.ToLower(finishReason.String()) {
			return true
		}
	}
	return false
}

func (s *Support[O, M]) IsProxyToolCalls(options function.Options, defaultOptions function.Options) bool {
	if options != nil {
		return options.ProxyToolCalls()
	}
	if defaultOptions != nil {
		return defaultOptions.ProxyToolCalls()
	}
	return false
}
