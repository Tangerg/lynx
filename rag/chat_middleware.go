package rag

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/samber/lo"
)

var (
	// ErrNilChatResponse reports a model that returned no response and no error.
	ErrNilChatResponse = errors.New("rag: chat model returned a nil response")
	// ErrNilChatStream reports a streamer that returned a nil sequence.
	ErrNilChatStream = errors.New("rag: chat streamer returned a nil sequence")
)

const (
	retrievedCandidatesMetadataKey = "rag/retrieved_candidates"
	citationsMetadataKey           = "rag/citations"
)

var historyValueKey = MustValueKey[[]chat.Message]("chat history")

// HistoryValueKey returns the immutable message snapshot slot populated
// when a query originates from [NewMiddleware].
func HistoryValueKey() ValueKey[[]chat.Message] { return historyValueKey }

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
	citations  []Citation
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

func (p preparedChatRequest) attachRetrievalMetadata(response *chat.Response) error {
	if response.Metadata == nil {
		response.Metadata = &chat.ResponseMetadata{}
	}
	if err := response.Metadata.Set(retrievedCandidatesMetadataKey, p.candidates); err != nil {
		return err
	}
	if len(p.citations) == 0 {
		delete(response.Metadata.Extra, citationsMetadataKey)
		return nil
	}
	return response.Metadata.Set(citationsMetadataKey, p.citations)
}

// RetrievedCandidates returns the candidates attached to response by
// [NewMiddleware]. The boolean reports whether retrieval metadata was present.
func RetrievedCandidates(response *chat.Response) ([]Candidate, bool, error) {
	if response == nil {
		return nil, false, ErrNilChatResponse
	}
	if response.Metadata == nil {
		return nil, false, nil
	}
	candidates, found, err := response.Metadata.Extra.Decode[[]Candidate](retrievedCandidatesMetadataKey)
	if err != nil {
		return nil, found, fmt.Errorf("rag: decode retrieved candidates: %w", err)
	}
	if found {
		if err := validateCandidates(candidates); err != nil {
			return nil, true, fmt.Errorf("rag: decode retrieved candidates: %w", err)
		}
	}
	return candidates, found, nil
}

// Citations returns the ordered citation mapping produced by the configured
// augmenter. The boolean reports whether citation metadata was present.
func Citations(response *chat.Response) ([]Citation, bool, error) {
	if response == nil {
		return nil, false, ErrNilChatResponse
	}
	if response.Metadata == nil {
		return nil, false, nil
	}
	citations, found, err := response.Metadata.Extra.Decode[[]Citation](citationsMetadataKey)
	if err != nil {
		return nil, found, fmt.Errorf("rag: decode citations: %w", err)
	}
	if found {
		if err := validateCitations(citations); err != nil {
			return nil, true, fmt.Errorf("rag: decode citations: %w", err)
		}
	}
	return citations, found, nil
}

// NewMiddleware builds call and stream middleware that retrieve documents before a chat
// request, augment the last user message, and attach retrieval and citation
// metadata to the response. Use [RetrievedCandidates] and [Citations] to read
// those typed values.
func NewMiddleware(config MiddlewareConfig) (chat.CallMiddleware, chat.StreamMiddleware, error) {
	if lo.IsNil(config.Retriever) {
		return nil, nil, ErrNilRetriever
	}
	if lo.IsNil(config.Augmenter) {
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

	query, err = query.WithValue(historyValueKey, slices.Clone(prepared.request.Messages))
	if err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: attach chat history: %w", err)
	}

	candidates, err := Retrieve(ctx, m.retriever, query)
	if err != nil {
		return preparedChatRequest{}, err
	}
	augmentation, err := m.augmenter.Augment(ctx, query, candidates)
	if err != nil {
		return preparedChatRequest{}, err
	}
	if err := augmentation.Validate(); err != nil {
		return preparedChatRequest{}, fmt.Errorf("rag: augmentation: %w", err)
	}
	prepared.candidates = candidates
	prepared.citations = augmentation.Citations()
	prepared.replaceFinalUserText(augmentation.Text())
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
	if extensionErr := prepared.attachRetrievalMetadata(resp); extensionErr != nil {
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
			if extensionErr := prepared.attachRetrievalMetadata(resp); extensionErr != nil {
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
	if lo.IsNil(next) {
		return nil
	}
	return chat.ModelFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

func (m *middleware) wrapStreamHandler(next chat.Streamer) chat.Streamer {
	if lo.IsNil(next) {
		return nil
	}
	return chat.StreamerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
