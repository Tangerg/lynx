// Package model defines the request/response handler primitives that every AI
// modality (chat, embedding, image, audio, moderation) builds on top of.
// The two core abstractions are [CallHandler] for synchronous request-response
// and [StreamHandler] for incremental output, both parameterized over arbitrary
// Request and Response types.
package model

import (
	"context"
	"iter"
)

// CallHandler executes a synchronous AI model request and returns the
// complete response, blocking until the model finishes or an error occurs.
//
// Use CallHandler when the caller needs the full result before continuing —
// single-turn Q&A, batch embeddings, image generation, classification, or
// function-call completion.
//
// Example:
//
//	resp, err := handler.Call(ctx, &chat.Request{Messages: msgs})
//	if err != nil {
//		return err
//	}
//	fmt.Println(resp.Result().AssistantMessage.Text)
type CallHandler[Request any, Response any] interface {
	// Call sends req to the underlying model and returns the complete
	// response. The call blocks until the model finishes or ctx is cancelled.
	Call(ctx context.Context, req Request) (Response, error)
}

// CallHandlerFunc adapts an ordinary function value to the [CallHandler]
// interface, mirroring [net/http.HandlerFunc].
type CallHandlerFunc[Request any, Response any] func(ctx context.Context, req Request) (Response, error)

// Call implements [CallHandler] by invoking the underlying function.
func (f CallHandlerFunc[Request, Response]) Call(ctx context.Context, req Request) (Response, error) {
	return f(ctx, req)
}

// StreamHandler executes an AI model request and returns an iterator over
// response chunks delivered as the model produces them. The iterator yields
// (chunk, nil) for each successful chunk and (zero, err) on the first error;
// callers terminate the stream by breaking out of the for-range loop or by
// cancelling ctx.
//
// Use StreamHandler for real-time chat, long-form generation, live
// transcription, or any case where reducing time-to-first-byte matters.
//
// Example:
//
//	for chunk, err := range handler.Stream(ctx, req) {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Print(chunk.Result().AssistantMessage.Text)
//	}
type StreamHandler[Request any, Response any] interface {
	// Stream sends req to the underlying model and returns an iterator over
	// response chunks. The iterator must be consumed in a single goroutine.
	Stream(ctx context.Context, req Request) iter.Seq2[Response, error]
}

// StreamHandlerFunc adapts an ordinary function value to the [StreamHandler]
// interface, mirroring [CallHandlerFunc].
type StreamHandlerFunc[Request any, Response any] func(ctx context.Context, req Request) iter.Seq2[Response, error]

// Stream implements [StreamHandler] by invoking the underlying function.
func (f StreamHandlerFunc[Request, Response]) Stream(ctx context.Context, req Request) iter.Seq2[Response, error] {
	return f(ctx, req)
}
