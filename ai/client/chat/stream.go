package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/result"
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

func (s *Streamer) Text(ctx context.Context) (stream.Reader[result.Result[string]], error) {
	resp, err := s.ChatResponse(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Map(resp, func(c result.Result[*chat.Response]) result.Result[string] {
		return result.Map(c, func(t *chat.Response) string {
			return t.Result().Output().Text()
		})
	}), nil
}

func (s *Streamer) ChatResponse(ctx context.Context) (stream.Reader[result.Result[*chat.Response]], error) {
	resp, err := s.Response(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Map(resp, func(c result.Result[*Response]) result.Result[*chat.Response] {
		return result.Map(c, func(t *Response) *chat.Response {
			return t.ChatResponse()
		})
	}), nil
}

func (s *Streamer) Response(ctx context.Context) (stream.Reader[result.Result[*Response]], error) {
	request, err := NewRequest(ctx, s.options)
	if err != nil {
		return nil, err
	}
	return s.Execute(request)
}

func (s *Streamer) Execute(ctx *Request) (stream.Reader[result.Result[*Response]], error) {
	invoker, err := newModelInvoker(ctx.chatModel)
	if err != nil {
		return nil, err
	}
	streamHandler := s.middleWares.makeStreamHandler(invoker)
	return streamHandler.Stream(ctx)
}
