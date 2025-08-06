package client

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/converter"
)

type Streamer struct {
	config            *Config
	middlewareManager *MiddlewareManager
}

func NewStreamer(config *Config) (*Streamer, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}

	return &Streamer{
		config:            config,
		middlewareManager: config.getMiddlewareManager(),
	}, nil
}

func (s *Streamer) execute(ctx context.Context, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	streamHandler := s.middlewareManager.makeStreamHandler(newInvoker(s.config.chatModel))
	return streamHandler.Stream(ctx, chatRequest)
}

func (s *Streamer) Execute(ctx context.Context, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		if chatRequest == nil {
			yield(nil, errors.New("chatRequest is required"))
			return
		}

		for chatResponse, executionErr := range s.execute(ctx, chatRequest) {
			if executionErr != nil {
				yield(nil, executionErr)
				return
			}

			if !yield(chatResponse, nil) {
				return
			}
		}
	}
}

// TODO Due to the streaming nature, all data needs to be aggregated before conversion. Conversion functionality is temporarily not provided until a more elegant approach is found.
func (s *Streamer) chatResponse(ctx context.Context, structuredConverter converter.StructuredConverter[any]) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		chatRequest, err := s.config.toChatRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		if structuredConverter != nil {
			chatRequest.Set(AttrOutputFormat.String(), structuredConverter.GetFormat())
		}

		for chatResponse, executionErr := range s.execute(ctx, chatRequest) {
			if executionErr != nil {
				yield(nil, executionErr)
				return
			}

			if !yield(chatResponse, nil) {
				return
			}
		}
	}

}

func (s *Streamer) ChatResponse(ctx context.Context) iter.Seq2[*chat.Response, error] {
	return s.chatResponse(ctx, nil)
}

func (s *Streamer) Text(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for chatResponse, streamErr := range s.chatResponse(ctx, nil) {
			if streamErr != nil {
				yield("", streamErr)
				return
			}

			responseText := chatResponse.Result().Output().Text()

			if !yield(responseText, nil) {
				return
			}
		}
	}
}
