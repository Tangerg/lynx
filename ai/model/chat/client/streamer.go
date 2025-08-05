package client

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/lynx/ai/model/chat"
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

func (s *Streamer) execute(ctx context.Context, chatModel chat.Model, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	streamHandler := s.middlewareManager.makeStreamHandler(newInvoker(chatModel))
	return streamHandler.Stream(ctx, chatRequest)
}

func (s *Streamer) Execute(ctx context.Context, chatModel chat.Model, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		if chatModel == nil {
			yield(nil, errors.New("chatModel is required"))
			return
		}

		if chatRequest == nil {
			yield(nil, errors.New("chatRequest is required"))
			return
		}

		for chatResponse, executionErr := range s.execute(ctx, chatModel, chatRequest) {
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
	return func(yield func(*chat.Response, error) bool) {
		chatRequest, err := s.config.toChatRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		for chatResponse, executionErr := range s.execute(ctx, s.config.chatModel, chatRequest) {
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

func (s *Streamer) Text(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for chatResponse, streamErr := range s.ChatResponse(ctx) {
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
