package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/pkg/mime"
)

// ModalityType labels the source content an embedding represents.
// Most callers see [Text]; image / audio / video embeddings are emitted
// by multimodal providers.
type ModalityType string

const (
	Text ModalityType = "text"
	Image ModalityType = "image"
	Audio ModalityType = "audio"
	Video ModalityType = "video"
)

func (m ModalityType) String() string { return string(m) }

// ResultMetadata holds per-embedding metadata: where in the input list
// the embedding came from, what kind of content produced it, and any
// provider extras.
type ResultMetadata struct {
	// Index is the position of this result in the input list.
	Index int64 `json:"index"`

	// ModalityType labels the source content type.
	ModalityType ModalityType `json:"modality_type"`

	// MimeType identifies the MIME type of the original content. nil
	// when the modality is plain text or the provider did not surface it.
	MimeType *mime.MIME `json:"mime_type,omitempty"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (m *ResultMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

func (m *ResultMetadata) Get(key string) (any, bool) {
	if m == nil || m.Extra == nil {
		return nil, false
	}
	value, exists := m.Extra[key]
	return value, exists
}

func (m *ResultMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Result is one embedding plus its metadata.
type Result struct {
	// Embedding is the vector representation of the input.
	Embedding []float64 `json:"embedding"`

	// Metadata carries the source position, modality, and any extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// Returns an error when the embedding is
// empty or metadata is nil.
func NewResult(embedding []float64, metadata *ResultMetadata) (*Result, error) {
	if len(embedding) == 0 {
		return nil, errors.New("embedding.NewResult: embedding vector must not be empty")
	}
	if metadata == nil {
		return nil, errors.New("embedding.NewResult: metadata must not be nil")
	}
	return &Result{Embedding: embedding, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata: the model actually
// used, token usage, rate-limit state, creation time, and provider
// extras.
type ResponseMetadata struct {
	// Model is the model name actually served.
	Model string `json:"model"`

	// Usage breaks down token consumption.
	Usage *model.Usage `json:"usage,omitempty"`

	// RateLimit reports quota state at request time.
	RateLimit *model.RateLimit `json:"rate_limit,omitempty"`

	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (m *ResponseMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

func (m *ResponseMetadata) Get(key string) (any, bool) {
	if m == nil || m.Extra == nil {
		return nil, false
	}
	value, exists := m.Extra[key]
	return value, exists
}

func (m *ResponseMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Response is the full embedding result: one [*Result] per input plus
// shared response metadata.
type Response struct {
	// Results holds one entry per input text, in the same order.
	Results []*Result `json:"results,omitzero"`

	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// from at least one result and a non-nil metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("embedding.NewResponse: at least one Result is required")
	}
	if metadata == nil {
		return nil, errors.New("embedding.NewResponse: metadata must not be nil")
	}
	return &Response{Results: results, Metadata: metadata}, nil
}

// Returns nil when Results is empty.
func (r *Response) Result() *Result {
	if r == nil || len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
