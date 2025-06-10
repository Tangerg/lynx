package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/pkg/stream"
)

type Streamer struct {
	options     *Options
	middleWares *Middlewares
}

func NewStreamer(options *Options) (*Streamer, error) {
	if options == nil {
		return nil, errors.New("options is required")
	}

	middleWares := options.middlewares
	if middleWares == nil {
		middleWares = NewMiddlewares()
	}

	return &Streamer{
		options:     options,
		middleWares: middleWares.Clone(),
	}, nil
}

func (s *Streamer) Text(ctx context.Context) (stream.Reader[string], error) {
	resp, err := s.ChatResponse(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Map(resp, func(c *response.ChatResponse) string {
		return c.Result().Output().Text()
	}), nil
}

func (s *Streamer) ChatResponse(ctx context.Context) (stream.Reader[*response.ChatResponse], error) {
	resp, err := s.Response(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Map(resp, func(c *Response) *response.ChatResponse {
		return c.ChatResponse()
	}), nil
}

func (s *Streamer) Response(ctx context.Context) (stream.Reader[*Response], error) {
	request, err := NewRequest(ctx, s.options)
	if err != nil {
		return nil, err
	}
	return s.Execute(request)
}

func (s *Streamer) Execute(ctx *Request) (stream.Reader[*Response], error) {
	invoker, err := newModelInvoker(ctx.ChatModel())
	if err != nil {
		return nil, err
	}
	streamHandler := s.middleWares.makeStreamHandler(invoker)
	return streamHandler.Stream(ctx)
}
