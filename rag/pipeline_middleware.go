package rag

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/chat"
)

const (
	// DocumentContextKey is the [chat.ResponseMetadata] key under which
	// the middleware stashes retrieved documents so downstream callers
	// can re-render or audit the context the LLM saw.
	DocumentContextKey = "lynx:ai:rag:document_context"

	// ChatHistoryKey is the [Query.Extra] key under which the
	// middleware threads the full message list, letting query
	// transformers and retrievers access conversational state.
	ChatHistoryKey = "lynx:ai:rag:chat_history"
)

// pipelineMiddleware wraps a RAG [Pipeline] as chat middleware: each
// call runs the pipeline on the user's last message, replaces it with
// the augmented form, and attaches the retrieved documents to the
// response metadata under [DocumentContextKey].
type pipelineMiddleware struct {
	pipeline *Pipeline
}

// NewPipelineMiddleware turns a [PipelineConfig] into the chat-side
// middleware pair. Returns an error when the underlying pipeline
// cannot be constructed.
//
// Example:
//
//	callMW, streamMW, err := rag.NewPipelineMiddleware(rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{vsRetriever},
//	    QueryAugmenter:     contextual,
//	})
//	resp, err := client.Chat().
//	    WithMiddlewares(callMW, streamMW).
//	    WithText("hi").
//	    Call().Response(ctx)
func NewPipelineMiddleware(config PipelineConfig) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	pipeline, err := NewPipeline(config)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.NewPipelineMiddleware: %w", err)
	}

	mw := &pipelineMiddleware{pipeline: pipeline}
	return mw.wrapCallHandler, mw.wrapStreamHandler, nil
}

// runPipeline executes the RAG pipeline against the request's last
// user message and returns the augmented query plus retrieved
// documents. Existing [Request.Params] are forwarded into the query's
// Extra map; the full message list is also threaded under
// [ChatHistoryKey] so transformers / retrievers can access it.
func (m *pipelineMiddleware) runPipeline(ctx context.Context, req *chat.Request) (*Query, []*document.Document, error) {
	query, err := NewQuery(req.UserMessage().Text)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.pipelineMiddleware.runPipeline: build query: %w", err)
	}

	for key, value := range req.Params {
		query.Set(key, value)
	}
	query.Set(ChatHistoryKey, req.Messages)

	augmented, docs, err := m.pipeline.Execute(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.pipelineMiddleware: pipeline failed: %w", err)
	}
	return augmented, docs, nil
}

// executeCall is the synchronous flow: run pipeline → swap user
// message → call next → attach docs to response metadata.
func (m *pipelineMiddleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	augmented, docs, err := m.runPipeline(ctx, req)
	if err != nil {
		return nil, err
	}

	req = withAugmentedUserText(req, augmented.Text)

	resp, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.Metadata.Set(DocumentContextKey, docs)
	return resp, nil
}

// executeStream is the streaming flow: run pipeline once before the
// stream begins, swap the user message, then forward chunks while
// attaching docs to each one's metadata.
func (m *pipelineMiddleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		augmented, docs, err := m.runPipeline(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		req = withAugmentedUserText(req, augmented.Text)

		for resp, err := range next.Stream(ctx, req) {
			if err != nil {
				yield(resp, err)
				return
			}
			resp.Metadata.Set(DocumentContextKey, docs)
			if !yield(resp, nil) {
				return
			}
		}
	}
}

// withAugmentedUserText returns a request whose last user message
// carries text instead of the original. It REPLACES the message in a
// fresh copy of the slice rather than mutating the existing
// *chat.UserMessage in place: that message pointer is shared with the
// caller's [chat.ClientRequest] (buildRequest only clones the slice,
// not its elements), so an in-place edit would corrupt the caller's
// stored messages and make a re-consumed stream augment its own
// already-augmented output. Media / Metadata on the original message
// are preserved.
func withAugmentedUserText(req *chat.Request, text string) *chat.Request {
	idx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if _, ok := req.Messages[i].(*chat.UserMessage); ok {
			idx = i
			break
		}
	}
	if idx == -1 {
		return req
	}

	out := *req
	out.Messages = slices.Clone(req.Messages)
	orig := req.Messages[idx].(*chat.UserMessage)
	out.Messages[idx] = &chat.UserMessage{
		Text:     text,
		Media:    orig.Media,
		Metadata: orig.Metadata,
	}
	return &out
}

// wrapCallHandler is the call-side adapter.
func (m *pipelineMiddleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler is the stream-side adapter.
func (m *pipelineMiddleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
