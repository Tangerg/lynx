package moderation

import "context"

type ClientCaller struct {
	request *ClientRequest
}

func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.request.
		MiddlewareChain().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

func (c *ClientCaller) Categories(ctx context.Context) (*Categories, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Categories, resp, nil
}

func (c *ClientCaller) AllCategories(ctx context.Context) ([]*Categories, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]*Categories, 0, len(resp.Results))
	for _, result := range resp.Results {
		out = append(out, result.Categories)
	}
	return out, resp, nil
}
