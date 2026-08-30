package rag

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
)

var (
	ErrNilChatResponse       = errors.New("rag: chat model returned a nil response")
	ErrNilChatStreamSequence = errors.New("rag: chat streamer returned a nil sequence")
	ErrNoFinalUserMessage    = errors.New("rag: chat request must end with a user message")
)

const (
	retrievedCandidatesMetadataKey = "rag/retrieved_candidates"
	citationsMetadataKey           = "rag/citations"
)

var historyValueKey = mustValueKey[[]chat.Message]("chat history")

// HistoryValueKey returns the immutable snapshot of messages before the active
// user turn when a query originates from [NewMiddleware].
func HistoryValueKey() ValueKey[[]chat.Message] { return historyValueKey }

type MiddlewareConfig struct {
	// Retriever fetches documents for the latest user message. Required.
	Retriever Retriever

	// Augmenter folds retrieved documents into the outgoing user message.
	// Required; use [IdentityAugmenter] for retrieval without prompt changes.
	Augmenter Augmenter
}

func (m MiddlewareConfig) validate() error {
	if lo.IsNil(m.Retriever) {
		return ErrNilRetriever
	}
	if lo.IsNil(m.Augmenter) {
		return ErrNilAugmenter
	}
	return nil
}

// Middleware owns retrieval and augmentation policy for both chat call modes.
// It is immutable after construction and safe for concurrent use when its
// Retriever and Augmenter are safe for concurrent use.
type Middleware struct {
	retriever Retriever
	augmenter Augmenter
}

type preparedChatRequest struct {
	request    *chat.Request
	candidates Candidates
	citations  Citations
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

func (p preparedChatRequest) finalUserText() (string, error) {
	last := len(p.request.Messages) - 1
	if last < 0 || p.request.Messages[last].Role != chat.RoleUser {
		return "", ErrNoFinalUserMessage
	}
	return p.request.Messages[last].Text(), nil
}

func (p *preparedChatRequest) replaceFinalUserText(text string) {
	index := len(p.request.Messages) - 1
	original := p.request.Messages[index]
	parts := make([]chat.Part, 0, len(original.Parts))
	replaced := false
	for partIndex := range original.Parts {
		switch original.Parts[partIndex].Kind {
		case chat.PartText:
			if !replaced {
				parts = append(parts, chat.NewTextPart(text))
				replaced = true
			}
		case chat.PartMedia:
			parts = append(parts, original.Parts[partIndex].Clone())
		}
	}
	p.request.Messages[index] = chat.Message{
		Role: chat.RoleUser, Parts: parts, Metadata: original.Metadata.Clone(),
	}
}

func (p preparedChatRequest) history() []chat.Message {
	history := make([]chat.Message, len(p.request.Messages)-1)
	for index := range history {
		history[index] = p.request.Messages[index].Clone()
	}
	return history
}

func (p preparedChatRequest) attachRetrievalMetadata(response *chat.Response) error {
	if response.Metadata == nil {
		response.Metadata = &chat.ResponseMetadata{}
	}
	if err := response.Metadata.Extra.Set(retrievedCandidatesMetadataKey, p.candidates); err != nil {
		return err
	}
	if len(p.citations) == 0 {
		delete(response.Metadata.Extra, citationsMetadataKey)
		return nil
	}
	return response.Metadata.Extra.Set(citationsMetadataKey, p.citations)
}

// CandidatesFromResponse returns the candidates attached to response by
// [NewMiddleware]. The boolean reports whether retrieval metadata was present.
func CandidatesFromResponse(response *chat.Response) (Candidates, bool, error) {
	if response == nil {
		return nil, false, ErrNilChatResponse
	}
	if response.Metadata == nil {
		return nil, false, nil
	}
	candidates, found, err := response.Metadata.Extra.Decode[Candidates](retrievedCandidatesMetadataKey)
	if err != nil {
		return nil, found, fmt.Errorf("rag: decode retrieved candidates: %w", err)
	}
	if found {
		if err := candidates.Validate(); err != nil {
			return nil, true, fmt.Errorf("rag: decode retrieved candidates: %w", err)
		}
	}
	return candidates, found, nil
}

// CitationsFromResponse returns the ordered citation mapping produced by the
// configured augmenter. The boolean reports whether citation metadata was
// present.
func CitationsFromResponse(response *chat.Response) (Citations, bool, error) {
	if response == nil {
		return nil, false, ErrNilChatResponse
	}
	if response.Metadata == nil {
		return nil, false, nil
	}
	citations, found, err := response.Metadata.Extra.Decode[Citations](citationsMetadataKey)
	if err != nil {
		return nil, found, fmt.Errorf("rag: decode citations: %w", err)
	}
	if found {
		if err := citations.Validate(); err != nil {
			return nil, true, fmt.Errorf("rag: decode citations: %w", err)
		}
	}
	return citations, found, nil
}

func NewMiddleware(config MiddlewareConfig) (*Middleware, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Middleware{retriever: config.Retriever, augmenter: config.Augmenter}, nil
}

func (m *Middleware) prepare(ctx context.Context, request *chat.Request) (preparedChatRequest, error) {
	prepared, err := newPreparedChatRequest(request)
	if err != nil {
		return preparedChatRequest{}, err
	}
	text, err := prepared.finalUserText()
	if err != nil {
		return preparedChatRequest{}, err
	}
	query, err := NewQuery(text)
	if err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: build query from final user message: %w", err)
	}

	query, err = query.WithValue(historyValueKey, prepared.history())
	if err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: attach chat history: %w", err)
	}

	candidates, err := Retrieve(ctx, m.retriever, query)
	if err != nil {
		return preparedChatRequest{}, err
	}
	augmentation, err := augment(ctx, m.augmenter, query, candidates)
	if err != nil {
		return preparedChatRequest{}, err
	}
	prepared.candidates = candidates
	prepared.citations = augmentation.Citations()
	prepared.replaceFinalUserText(augmentation.Text())
	return prepared, nil
}

func (m *Middleware) call(ctx context.Context, request *chat.Request, next chat.Model) (*chat.Response, error) {
	prepared, err := m.prepare(ctx, request)
	if err != nil {
		return nil, err
	}

	response, err := next.Call(ctx, prepared.request)
	if response == nil {
		if err != nil {
			return nil, err
		}
		return nil, ErrNilChatResponse
	}
	if extensionErr := prepared.attachRetrievalMetadata(response); extensionErr != nil {
		return response, errors.Join(err, extensionErr)
	}
	return response, err
}

func (m *Middleware) stream(ctx context.Context, request *chat.Request, next chat.Streamer) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		prepared, err := m.prepare(ctx, request)
		if err != nil {
			yield(nil, err)
			return
		}

		sequence := next.Stream(ctx, prepared.request)
		if sequence == nil {
			yield(nil, ErrNilChatStreamSequence)
			return
		}
		for response, streamErr := range sequence {
			if response == nil {
				if streamErr != nil {
					yield(nil, streamErr)
					return
				}
				yield(nil, ErrNilChatResponse)
				return
			}
			if extensionErr := prepared.attachRetrievalMetadata(response); extensionErr != nil {
				yield(response, errors.Join(streamErr, extensionErr))
				return
			}
			if streamErr != nil {
				yield(response, streamErr)
				return
			}
			if !yield(response, nil) {
				return
			}
		}
	}
}

func (m *Middleware) Call(next chat.Model) chat.Model {
	if lo.IsNil(next) {
		return nil
	}
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		return m.call(ctx, request, next)
	})
}

func (m *Middleware) Stream(next chat.Streamer) chat.Streamer {
	if lo.IsNil(next) {
		return nil
	}
	return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.stream(ctx, request, next)
	})
}
