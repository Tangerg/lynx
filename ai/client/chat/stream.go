package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/pkg/stream"
)

type Stream struct {
	request     *Request
	middleWares *Middlewares
}

func NewStream(request *Request, middleWares ...*Middlewares) (*Stream, error) {
	if request == nil {
		return nil, errors.New("request is required")
	}
	var md *Middlewares
	if len(middleWares) > 0 &&
		middleWares[0] != nil {
		md = middleWares[0]
	} else {
		md = NewMiddlewares()
	}

	return &Stream{
		request:     request,
		middleWares: md.Clone(),
	}, nil
}

func (s *Stream) Text(ctx context.Context) (stream.Reader[string], error) {
	resp, err := s.ChatResponse(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Map(resp, func(c *response.ChatResponse) string {
		return c.Result().Output().Text()
	}), nil
}

func (s *Stream) ChatResponse(ctx context.Context) (stream.Reader[*response.ChatResponse], error) {
	resp, err := s.Response(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Map(resp, func(c *Response) *response.ChatResponse {
		return c.ChatResponse()
	}), nil
}

func (s *Stream) Response(ctx context.Context) (stream.Reader[*Response], error) {
	return s.do(newContextFromRequest(ctx, s.request))
}

func (s *Stream) do(ctx *Context) (stream.Reader[*Response], error) {
	invoker := newModelInvoker(s.request.chatModel)
	streamHandler := s.middleWares.makeStreamHandler(invoker)
	return streamHandler.Stream(ctx)
}
