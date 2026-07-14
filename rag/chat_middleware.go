package rag

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
)

// DocumentContextKey is the [chat.Response.Extensions] key under which the
// middleware stashes retrieved documents so downstream callers can re-render or
// audit the context the LLM saw.
const DocumentContextKey = "rag/document_context"

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
// response extensions under [DocumentContextKey].
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
	userText := ""
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == chat.RoleUser {
			userText = req.Messages[index].Text()
			break
		}
	}
	query, err := NewQuery(userText)
	if err != nil {
		return nil, nil, fmt.Errorf("rag.NewMiddleware: build query: %w", err)
	}

	values, err := req.Extensions.Values()
	if err != nil {
		return nil, nil, fmt.Errorf("rag.NewMiddleware: decode request extensions: %w", err)
	}
	for key, value := range values {
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
// docs to response extensions.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.Model) (*chat.Response, error) {
	augmented, docs, err := m.run(ctx, req)
	if err != nil {
		return nil, err
	}

	req = withAugmentedUserText(req, augmented.Text)

	resp, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := resp.SetExtension(DocumentContextKey, docs); err != nil {
		return nil, err
	}
	return resp, nil
}

// executeStream is the streaming flow: retrieve once before the stream begins,
// swap the user message, then forward chunks while attaching docs to extensions.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.Streamer) iter.Seq2[*chat.Response, error] {
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
			if err := resp.SetExtension(DocumentContextKey, docs); err != nil {
				yield(nil, err)
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

// withAugmentedUserText returns a shallow request copy with a fresh message
// slice and a replacement for the final user message. The caller's protocol
// values remain unchanged, while user media and message metadata are retained.
func withAugmentedUserText(req *chat.Request, text string) *chat.Request {
	idx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == chat.RoleUser {
			idx = i
			break
		}
	}
	if idx == -1 {
		return req
	}

	out := *req
	out.Messages = slices.Clone(req.Messages)
	original := req.Messages[idx]
	parts := make([]chat.Part, 0, 1+len(original.Parts))
	if text != "" {
		parts = append(parts, chat.NewTextPart(text))
	}
	for index := range original.Parts {
		if original.Parts[index].Kind == chat.PartMedia {
			parts = append(parts, original.Parts[index])
		}
	}
	out.Messages[idx] = chat.Message{
		Role:     chat.RoleUser,
		Parts:    parts,
		Metadata: original.Metadata.Clone(),
	}
	return &out
}

func (m *middleware) wrapCallHandler(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

func (m *middleware) wrapStreamHandler(next chat.Streamer) chat.Streamer {
	return chat.StreamerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
