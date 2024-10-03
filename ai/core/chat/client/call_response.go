package client

import (
	"context"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/completion"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/converter"
)

type CallResponseSpec interface {
	Entity(ctx context.Context, v any) error
	EntityByConvert(ctx context.Context, v any, converter converter.StructuredConverter[any]) error
	Content(ctx context.Context) (string, error)
	ChatResponse(ctx context.Context) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error)
}

var _ CallResponseSpec = (*DefaultCallResponseSpec)(nil)

type DefaultCallResponseSpec struct {
	request *DefaultChatClientRequest
}

func NewDefaultCallResponseSpec(req *DefaultChatClientRequest) *DefaultCallResponseSpec {
	return &DefaultCallResponseSpec{
		request: req,
	}
}

func (d *DefaultCallResponseSpec) doGetChatResponse(ctx context.Context, request *DefaultChatClientRequest, format string) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error) {
	c := api.NewContext(ctx)
	if format != "" {
		c.SetParam("formatParam", format)
	}
	c.SetParams(request.advisorParams)
	c.Request = request.ToAdvisedRequest()

	reqAdvisors := api.ExtractRequestAdvisor(request.advisors)
	for _, reqAdvisor := range reqAdvisors {
		err := reqAdvisor.AdviseRequest(c)
		if err != nil {
			return nil, err
		}
	}

	err := request.aroundAdvisorChain.NextAroundCall(c)
	if err != nil {
		return nil, err
	}

	respAdvisors := api.ExtractResponseAdvisor(request.advisors)
	for _, respAdvisor := range respAdvisors {
		err = respAdvisor.AdviseCallResponse(c)
		if err != nil {
			return nil, err
		}
	}
	return c.Response, nil
}

func (d *DefaultCallResponseSpec) EntityByConvert(ctx context.Context, v any, converter converter.StructuredConverter[any]) error {
	resp, err := d.doGetChatResponse(ctx, d.request, converter.GetFormat())
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

func (d *DefaultCallResponseSpec) Entity(ctx context.Context, v any) error {
	c := new(converter.StructConverter[any])
	c.SetV(v)
	return d.EntityByConvert(ctx, v, c)
}

func (d *DefaultCallResponseSpec) Content(ctx context.Context) (string, error) {
	resp, err := d.ChatResponse(ctx)
	if err != nil {
		return "", err
	}
	return resp.Result().Output().Content(), nil
}

func (d *DefaultCallResponseSpec) ChatResponse(ctx context.Context) (*completion.ChatCompletion[metadata.ChatGenerationMetadata], error) {
	return d.doGetChatResponse(ctx, d.request, "")
}
