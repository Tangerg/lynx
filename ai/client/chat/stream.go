package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/stream"
)

type Streamer struct {
	session           *Session
	middlewareManager *MiddlewareManager
}

func NewStreamer(session *Session) (*Streamer, error) {
	if session == nil {
		return nil, errors.New("session is required")
	}

	middlewareManager := session.middlewareManager
	if middlewareManager == nil {
		middlewareManager = NewMiddlewareManager()
	}

	return &Streamer{
		session:           session,
		middlewareManager: middlewareManager.Clone(),
	}, nil
}

func (s *Streamer) Text(ctx context.Context) (stream.Reader[result.Result[string]], error) {
	chatResponseStream, err := s.ChatResponse(ctx)
	if err != nil {
		return nil, err
	}

	return stream.Map(chatResponseStream, func(chatResult result.Result[*chat.Response]) result.Result[string] {
		return result.Map(chatResult, func(chatResponse *chat.Response) string {
			return chatResponse.Result().Output().Text()
		})
	}), nil
}

func (s *Streamer) ChatResponse(ctx context.Context) (stream.Reader[result.Result[*chat.Response]], error) {
	responseStream, err := s.Response(ctx)
	if err != nil {
		return nil, err
	}

	return stream.Map(responseStream, func(responseResult result.Result[*Response]) result.Result[*chat.Response] {
		return result.Map(responseResult, func(response *Response) *chat.Response {
			return response.ChatResponse()
		})
	}), nil
}

func (s *Streamer) Response(ctx context.Context) (stream.Reader[result.Result[*Response]], error) {
	request, err := NewRequest(ctx, s.session)
	if err != nil {
		return nil, err
	}

	return s.Execute(request)
}

func (s *Streamer) Execute(request *Request) (stream.Reader[result.Result[*Response]], error) {
	invoker, err := newModelInvoker(request.chatModel)
	if err != nil {
		return nil, err
	}

	streamHandler := s.middlewareManager.makeStreamHandler(invoker)
	return streamHandler.Stream(request)
}
