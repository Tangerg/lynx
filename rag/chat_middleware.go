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

type preparedChatRequest struct {
	request    *chat.Request
	candidates []Candidate
}

func newPreparedChatRequest(request *chat.Request) (preparedChatRequest, error) {
	if request == nil {
		return preparedChatRequest{}, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	clone := request.Clone()
	if err := clone.Validate(); err != nil {
		return preparedChatRequest{}, err
	}
	return preparedChatRequest{request: clone}, nil
}

func (p preparedChatRequest) finalUserText() string {
	for index := len(p.request.Messages) - 1; index >= 0; index-- {
		if p.request.Messages[index].Role == chat.RoleUser {
			return p.request.Messages[index].Text()
		}
	}
	return ""
}

func (p *preparedChatRequest) replaceFinalUserText(text string) {
	for index := len(p.request.Messages) - 1; index >= 0; index-- {
		if p.request.Messages[index].Role != chat.RoleUser {
			continue
		}
		original := p.request.Messages[index]
		parts := make([]chat.Part, 0, 1+len(original.Parts))
		if text != "" {
			parts = append(parts, chat.NewTextPart(text))
		}
		for partIndex := range original.Parts {
			if original.Parts[partIndex].Kind == chat.PartMedia {
				parts = append(parts, original.Parts[partIndex])
			}
		}
		p.request.Messages[index] = chat.Message{
			Role:     chat.RoleUser,
			Parts:    parts,
			Metadata: original.Metadata.Clone(),
		}
		return
	}
}

func (p preparedChatRequest) attachDocuments(response *chat.Response) error {
	if response.Metadata == nil {
		response.Metadata = &chat.ResponseMetadata{}
	}
	return response.Metadata.Set(DocumentContextKey, p.candidates)
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

func (m *middleware) prepare(ctx context.Context, request *chat.Request) (preparedChatRequest, error) {
	prepared, err := newPreparedChatRequest(request)
	if err != nil {
		return preparedChatRequest{}, err
	}
	query, err := NewQuery(prepared.finalUserText())
	if err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: build query from final user message: %w", err)
	}

	if len(prepared.request.Options.Extensions) != 0 {
		query, err = query.WithValue(requestOptionsExtensionsValueKey, prepared.request.Options.Extensions.Clone())
		if err != nil {
			return preparedChatRequest{}, fmt.Errorf("rag: attach request Options extensions: %w", err)
		}
	}
	query, err = query.WithValue(chatHistoryValueKey, slices.Clone(prepared.request.Messages))
	if err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: attach chat history: %w", err)
	}

	candidates, err := Retrieve(ctx, m.retriever, query)
	if err != nil {
		return preparedChatRequest{}, err
	}
	augmented, err := m.augmenter.Augment(ctx, query, candidates)
	if err != nil {
		return preparedChatRequest{}, err
	}
	if err := augmented.Validate(); err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: augmented query: %w", err)
	}
	prepared.candidates = candidates
	prepared.replaceFinalUserText(augmented.Text())
	return prepared, nil
}

// executeCall is the synchronous flow: retrieve → augment → call next → attach
// docs to response metadata.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.Model) (*chat.Response, error) {
	prepared, err := m.prepare(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := next.Call(ctx, prepared.request)
	if resp == nil {
		if err != nil {
			return nil, err
		}
		return nil, ErrNilChatResponse
	}
	if extensionErr := prepared.attachDocuments(resp); extensionErr != nil {
		return resp, errors.Join(err, extensionErr)
	}
	return resp, err
}

// executeStream is the streaming flow: retrieve once before the stream begins,
// swap the user message, then forward chunks while attaching docs to response
// metadata.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.Streamer) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		prepared, err := m.prepare(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		sequence := next.Stream(ctx, prepared.request)
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
			if extensionErr := prepared.attachDocuments(resp); extensionErr != nil {
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
