package rag

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
)

// DocumentContextKey is the [chat.ResponseMetadata] key under which the
// middleware stashes retrieved documents so downstream callers can re-render or
// audit the context the LLM saw.
const DocumentContextKey = "lynx:ai:rag:document_context"

type MiddlewareConfig struct {
	// Retriever fetches documents for the latest user message. Required.
	Retriever Retriever

	// Augmenter folds retrieved documents into the outgoing user message.
	// Nil means [IdentityAugmenter].
	Augmenter Augmenter
}

type middleware struct {
	retriever Retriever
	augmenter Augmenter
}

// NewMiddleware builds call and stream middleware that retrieve documents before a chat
// request, augment the last user message, and attach retrieved documents to
// response metadata under [DocumentContextKey].
func NewMiddleware(config MiddlewareConfig) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	if config.Retriever == nil {
		return nil, nil, ErrNilRetriever
	}
	if config.Augmenter == nil {
		config.Augmenter = IdentityAugmenter()
	}

	mw := &middleware{retriever: config.Retriever, augmenter: config.Augmenter}
	return mw.wrapCallHandler, mw.wrapStreamHandler, nil
}

func (m *middleware) run(ctx context.Context, req *chat.Request) (*Query, []Candidate, error) {
	query, err := NewQuery(req.UserMessage().Text)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.NewMiddleware: build query: %w", err)
	}

	for key, value := range req.Params {
		query.Set(key, value)
	}
	query.Set(ChatHistoryKey, req.Messages)

	docs, err := m.retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	augmented, err := m.augmenter.Augment(ctx, query, docs)
	if err != nil {
		return nil, nil, err
	}
	return augmented, docs, nil
}

// executeCall is the synchronous flow: retrieve → augment → call next → attach
// docs to response metadata.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	augmented, docs, err := m.run(ctx, req)
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

// executeStream is the streaming flow: retrieve once before the stream begins,
// swap the user message, then forward chunks while attaching docs to metadata.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		augmented, docs, err := m.run(ctx, req)
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

func (m *middleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

func (m *middleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
