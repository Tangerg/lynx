package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/converter"
)

type CallResponse interface {
	Entity(ctx context.Context, v any) error
	EntityByConvert(ctx context.Context, v any, converter converter.StructuredConverter[any]) error
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error)
}

var _ CallResponse = (*DefaultCallResponse)(nil)

type DefaultCallResponse struct {
	request *DefaultChatClientRequest
}

func NewDefaultCallResponseSpec(req *DefaultChatClientRequest) *DefaultCallResponse {
	return &DefaultCallResponse{
		request: req,
	}
}

func (d *DefaultCallResponse) doGetChatResponse(ctx context.Context, format string) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error) {
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

	err := d.request.aroundAdvisorChain.NextAroundCall(c)
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

func (d *DefaultCallResponse) Entity(ctx context.Context, v any) error {
	c := new(converter.StructConverter[any])
	c.SetV(v)
	return d.EntityByConvert(ctx, v, c)
}

func (d *DefaultCallResponse) EntityByConvert(ctx context.Context, v any, converter converter.StructuredConverter[any]) error {
	resp, err := d.doGetChatResponse(ctx, converter.GetFormat())
	if err != nil {
		return err
	}
	convert, err := converter.Convert(resp.Result().Output().Content())
	if err != nil {
		return err
	}
	v = convert
	return nil
}

func (d *DefaultCallResponse) Content(ctx context.Context) (string, error) {
	resp, err := d.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Content(), nil
}

func (d *DefaultCallResponse) ChatResponse(ctx context.Context) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error) {
	return d.doGetChatResponse(ctx, "")
}
