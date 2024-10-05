package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type StreamResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error)
}

var _ StreamResponse[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultStreamResponse[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultStreamResponse[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func NewDefaultStreamResponseSpec[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](req *DefaultChatClientRequest[O, M]) *DefaultStreamResponse[O, M] {
	return &DefaultStreamResponse[O, M]{
		request: req,
	}
}

func (d *DefaultStreamResponse[O, M]) doGetChatResponse(ctx context.Context, format string) (*completion.ChatCompletion[M], error) {
	c := api.NewContext[O, M](ctx)
	if format != "" {
		c.SetParam("formatParam", format)
	}
	c.SetParams(d.request.advisorParams)
	c.Request = d.request.toAdvisedRequest()

	reqAdvisors := api.ExtractRequestAdvisor[O, M](d.request.advisors)
	for _, reqAdvisor := range reqAdvisors {
		err := reqAdvisor.AdviseRequest(c)
		if err != nil {
			return nil, err
		}
	}

	err := d.request.aroundAdvisorChain.NextAroundStream(c)
	if err != nil {
		return nil, err
	}

	respAdvisors := api.ExtractResponseAdvisor[O, M](d.request.advisors)
	for _, respAdvisor := range respAdvisors {
		err = respAdvisor.AdviseCallResponse(c)
		if err != nil {
			return nil, err
		}
	}

	return c.Response, nil
}

func (d *DefaultStreamResponse[O, M]) Content(ctx context.Context) (string, error) {
	resp, err := d.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Content(), nil
}

func (d *DefaultStreamResponse[O, M]) ChatResponse(ctx context.Context) (*completion.ChatCompletion[M], error) {
	return d.doGetChatResponse(ctx, "")
}
