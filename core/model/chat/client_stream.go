package chat

import (
	"context"
	"iter"
)

// ClientStreamer drives the streaming chat path. Build it via
// [ClientRequest.Stream]; consume it via [ClientStreamer.Response] or
// [ClientStreamer.Text].
type ClientStreamer struct {
	request *ClientRequest
}

// stream feeds the request through the middleware chain into the model.
// Tool execution is NOT auto-injected; register the call/stream middleware
// pair for your loop driver via WithCallMiddlewares and WithStreamMiddlewares
// if you need that.
func (s *ClientStreamer) stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	handler := s.request.MiddlewareChain().BuildStreamHandler(s.request.model)
	return handler.Stream(ctx, req)
}

// runStream is the shared entry point for streaming: build the request,
// optionally inject parser instructions, then run the middleware chain.
// Structured parsing on streams is not yet implemented because the
// parser requires the full text and stream provides incremental chunks.
func (s *ClientStreamer) runStream(ctx context.Context, parser StructuredParser[any]) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		req, err := s.request.buildRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		if parser != nil {
			req.AppendToLastUserMessage(parser.Instructions())
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

func (s *ClientStreamer) Response(ctx context.Context) iter.Seq2[*Response, error] {
	return s.runStream(ctx, nil)
}

// Text streams just the assistant's text deltas, convenient when you
// want to write directly to a UI buffer without unpacking the response.
func (s *ClientStreamer) Text(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for resp, err := range s.runStream(ctx, nil) {
			if err != nil {
				yield("", err)
				return
			}
			if !yield(resp.TextDelta(), nil) {
				return
			}
		}
	}
}
