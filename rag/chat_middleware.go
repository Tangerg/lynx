package rag

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrNilChatResponse reports a model that returned no response and no error.
	ErrNilChatResponse = errors.New("rag: chat model returned a nil response")
	// ErrNilChatStream reports a streamer that returned a nil sequence.
	ErrNilChatStream = errors.New("rag: chat streamer returned a nil sequence")
)

// DocumentContextKey is the [chat.ResponseMetadata.Extra] key under which the
// middleware stashes retrieved documents so downstream callers can re-render or
// audit the context the LLM saw.
const DocumentContextKey = "rag/document_context"

var (
	chatHistoryValueKey              = MustValueKey[[]chat.Message]("chat history")
	requestOptionsExtensionsValueKey = MustValueKey[metadata.Map]("chat request options extensions")
)

// ChatHistoryValueKey returns the immutable message snapshot slot populated
// when a query originates from [NewMiddleware].
func ChatHistoryValueKey() ValueKey[[]chat.Message] { return chatHistoryValueKey }

// RequestOptionsExtensionsValueKey returns the request Options extension
// envelope slot. The envelope remains separate from the RAG value namespace
// rather than being flattened into a weakly typed map.
func RequestOptionsExtensionsValueKey() ValueKey[metadata.Map] {
	return requestOptionsExtensionsValueKey
}

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
// [chat.ResponseMetadata.Extra] under [DocumentContextKey].
func NewMiddleware(config MiddlewareConfig) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	if isNil(config.Retriever) {
		return nil, nil, ErrNilRetriever
	}
	if isNil(config.Augmenter) {
		config.Augmenter = IdentityAugmenter()
	}

	mw := &middleware{retriever: config.Retriever, augmenter: config.Augmenter}
	return mw.wrapCallHandler, mw.wrapStreamHandler, nil
}

func (m *middleware) run(ctx context.Context, req *chat.Request) (*chat.Request, *Query, []Candidate, error) {
	if req == nil {
		return nil, nil, nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	prepared := req.Clone()
	if err := prepared.Validate(); err != nil {
		return nil, nil, nil, err
	}
	userText := ""
	for index := len(prepared.Messages) - 1; index >= 0; index-- {
		if prepared.Messages[index].Role == chat.RoleUser {
			userText = prepared.Messages[index].Text()
			break
		}
	}
	query, err := NewQuery(userText)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("rag: build query from final user message: %w", err)
	}

	if len(prepared.Options.Extensions) != 0 {
		query, err = WithValue(query, requestOptionsExtensionsValueKey, prepared.Options.Extensions.Clone())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("rag: attach request Options extensions: %w", err)
		}
	}
	query, err = WithValue(query, chatHistoryValueKey, slices.Clone(prepared.Messages))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("rag: attach chat history: %w", err)
	}

	docs, err := m.retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, nil, nil, err
	}
	for index, candidate := range docs {
		if err := candidate.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("rag: retrieved candidate %d: %w", index, err)
		}
	}
	augmented, err := m.augmenter.Augment(ctx, query, docs)
	if err != nil {
		return nil, nil, nil, err
	}
	if augmented == nil {
		return nil, nil, nil, ErrNilQuery
	}
	return prepared, augmented, docs, nil
}

// executeCall is the synchronous flow: retrieve → augment → call next → attach
// docs to response metadata.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.Model) (*chat.Response, error) {
	prepared, augmented, docs, err := m.run(ctx, req)
	if err != nil {
		return nil, err
	}

	prepared = withAugmentedUserText(prepared, augmented.Text())

	resp, err := next.Call(ctx, prepared)
	if resp == nil {
		if err != nil {
			return nil, err
		}
		return nil, ErrNilChatResponse
	}
	if resp.Metadata == nil {
		resp.Metadata = &chat.ResponseMetadata{}
	}
	if extensionErr := resp.Metadata.Set(DocumentContextKey, docs); extensionErr != nil {
		return resp, errors.Join(err, extensionErr)
	}
	return resp, err
}

// executeStream is the streaming flow: retrieve once before the stream begins,
// swap the user message, then forward chunks while attaching docs to response
// metadata.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.Streamer) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		prepared, augmented, docs, err := m.run(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		prepared = withAugmentedUserText(prepared, augmented.Text())

		sequence := next.Stream(ctx, prepared)
		if sequence == nil {
			yield(nil, ErrNilChatStream)
			return
		}
		for resp, err := range sequence {
			if resp == nil {
				if err != nil {
					yield(nil, err)
					return
				}
				yield(nil, ErrNilChatResponse)
				return
			}
			if resp.Metadata == nil {
				resp.Metadata = &chat.ResponseMetadata{}
			}
			if extensionErr := resp.Metadata.Set(DocumentContextKey, docs); extensionErr != nil {
				yield(resp, errors.Join(err, extensionErr))
				return
			}
			if err != nil {
				yield(resp, err)
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
	if isNil(next) {
		return nil
	}
	return chat.ModelFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

func (m *middleware) wrapStreamHandler(next chat.Streamer) chat.Streamer {
	if isNil(next) {
		return nil
	}
	return chat.StreamerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
