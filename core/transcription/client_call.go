package transcription

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

func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return "", nil, err
	}
	return resp.Result.Text, resp, nil
}
