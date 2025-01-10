package chat

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/function"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

type helper struct {
	function.Support[*OpenAIChatRequestOptions, *OpenAIChatResultMetadata]
}

func newHelper() *helper {
	return &helper{}
}

func (h *helper) shouldHandleToolCalls(defaultOptions *OpenAIChatRequestOptions, req *OpenAIChatRequest, resp *OpenAIChatResponse) bool {
	if h.IsProxyToolCalls(req.Options(), defaultOptions) {
		return false
	}
	return h.IsToolCallChatResponse(resp, []result.FinishReason{result.ToolCalls, result.Stop})
}

func (h *helper) handleToolCalls(ctx context.Context, req *OpenAIChatRequest, resp *OpenAIChatResponse) (*OpenAIChatRequest, error) {
	msgs, err := h.HandleToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	newReq, _ := newOpenAIChatRequestBuilder().
		WithOptions(req.Options()).
		WithMessages(msgs...).
		Build()

	return newReq, nil
}
