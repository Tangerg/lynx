package tts

import (
	"context"
	"iter"
)

type ClientStreamer struct {
	request *ClientRequest
}

func (s *ClientStreamer) stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return s.request.
		MiddlewareChain().
		BuildStreamHandler(s.request.model).
		Stream(ctx, req)
}

func (s *ClientStreamer) Response(ctx context.Context) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		req, err := s.request.buildRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		for resp, streamErr := range s.stream(ctx, req) {
			if streamErr != nil {
				yield(nil, streamErr)
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

// Speech yields just the audio bytes — convenient when the caller wants
// to pipe directly to a player or file.
func (s *ClientStreamer) Speech(ctx context.Context) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for resp, err := range s.Response(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(resp.Result.Speech, nil) {
				return
			}
		}
	}
}
