package function

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/function"
)

func NewSupport[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](options function.Options, funcs ...function.Function) *Support[O, M] {
	s := &Support[O, M]{
		functions: make(map[string]function.Function),
	}
	s.RegisterFunctions(s.Merage(options, funcs...)...)
	return s
}

type Support[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	mu        sync.RWMutex
	functions map[string]function.Function
}

func (s *Support[O, M]) Functions() map[string]function.Function {
	return s.functions
}

func (s *Support[O, M]) Merage(options function.Options, funcs ...function.Function) []function.Function {
	rv := make([]function.Function, 0)
	rv = append(rv, funcs...)
	rv = append(rv, options.Functions()...)
	options.SetFunctions([]function.Function{})
	return rv
}

func (s *Support[O, M]) RegisterFunctions(funcs ...function.Function) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, f := range funcs {
		s.functions[f.Name()] = f
	}
}

func (s *Support[O, M]) LookupFunctions(names ...string) []function.Function {
	names = lo.Uniq(names)

	s.mu.RLock()
	defer s.mu.RUnlock()

	rv := make([]function.Function, 0, len(names))

	for _, name := range names {
		f, ok := s.functions[name]
		if ok {
			rv = append(rv, f)
		}
	}

	return rv
}

func (s *Support[O, M]) HandleToolCalls(ctx context.Context, p *prompt.ChatPrompt[O], chatResp *completion.ChatCompletion[M]) ([]message.ChatMessage, error) {
	var toolCallGeneration model.Result[*message.AssistantMessage, M]
	for _, result := range chatResp.Results() {
		if result.Output().HasToolCalls() {
			toolCallGeneration = result
			break
		}
	}
	if toolCallGeneration == nil {
		return nil, errors.New("no tool call generation found in the response")
	}
	toolMessage, err := s.ExecuteFunctions(ctx, toolCallGeneration.Output())
	if err != nil {
		return nil, err
	}
	return s.BuildToolCallConversation(
		p.Instructions(),
		toolCallGeneration.Output(),
		toolMessage,
	), nil
}

func (s *Support[O, M]) BuildToolCallConversation(msgs []message.ChatMessage, assistantMessage *message.AssistantMessage, toolMessage *message.ToolMessage) []message.ChatMessage {
	rv := make([]message.ChatMessage, 0, len(msgs)+2)
	copy(rv, msgs)
	rv = append(rv, assistantMessage, toolMessage)
	return rv
}

func (s *Support[O, M]) ExecuteFunctions(ctx context.Context, assistantMessage *message.AssistantMessage) (*message.ToolMessage, error) {
	resps := make([]*message.ToolResponse, 0, len(assistantMessage.ToolCalls()))
	for _, toolCall := range assistantMessage.ToolCalls() {
		f, ok := s.functions[toolCall.Name]
		if !ok {
			return nil, fmt.Errorf("no function callback found for function name: %s", toolCall.Name)
		}
		resp, err := f.Call(ctx, toolCall.Arguments)
		if err != nil {
			return nil, err
		}
		resps = append(resps, &message.ToolResponse{
			ID:   toolCall.ID,
			Name: toolCall.Name,
			Data: resp,
		})
	}
	return message.NewToolMessage(resps), nil
}

func (s *Support[O, M]) IsToolCallChatCompletion(chatResp *completion.ChatCompletion[M], finishReasons []metadata.FinishReason) bool {
	for _, result := range chatResp.Results() {
		if s.IsToolCallChatGeneration(result, finishReasons) {
			return true
		}
	}
	return false
}

func (s *Support[O, M]) IsToolCallChatGeneration(gen model.Result[*message.AssistantMessage, M], finishReasons []metadata.FinishReason) bool {
	if !gen.Output().HasToolCalls() {
		return false
	}
	reason := gen.Metadata().FinishReason()
	for _, finishReason := range finishReasons {
		if strings.ToLower(reason.String()) == strings.ToLower(finishReason.String()) {
			return true
		}
	}
	return false
}

func (s *Support[O, M]) IsProxyToolCalls(options function.Options, defaultOptions function.Options) bool {
	if options != nil {
		return options.UseProxy()
	}
	if defaultOptions != nil {
		return defaultOptions.UseProxy()
	}
	return false
}
