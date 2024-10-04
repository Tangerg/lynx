package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
)

type StreamResponse interface {
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error)
}

var _ StreamResponse = (*DefaultStreamResponse)(nil)

type DefaultStreamResponse struct {
	request *DefaultChatClientRequest
}

func NewDefaultStreamResponseSpec(req *DefaultChatClientRequest) *DefaultStreamResponse {
	return &DefaultStreamResponse{
		request: req,
	}
}

func (d *DefaultStreamResponse) doGetChatResponse(ctx context.Context, format string) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error) {
	c := api.NewContext(ctx)
	if format != "" {
		c.SetParam("formatParam", format)
	}
	c.SetParams(d.request.advisorParams)
	c.Request = d.request.toAdvisedRequest()

	reqAdvisors := api.ExtractRequestAdvisor(d.request.advisors)
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

	respAdvisors := api.ExtractResponseAdvisor(d.request.advisors)
	for _, respAdvisor := range respAdvisors {
		err = respAdvisor.AdviseCallResponse(c)
		if err != nil {
			return nil, err
		}
	}

	return c.Response, nil
}

func (d *DefaultStreamResponse) Content(ctx context.Context) (string, error) {
	resp, err := d.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Content(), nil
}

func (d *DefaultStreamResponse) ChatResponse(ctx context.Context) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error) {
	return d.doGetChatResponse(ctx, "")
}
