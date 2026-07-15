package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/core/metadata"
)

// ModalityType labels the source content an embedding represents.
// Most callers see [Text]; image / audio / video embeddings are emitted
// by multimodal providers.
type ModalityType string

const (
	Text  ModalityType = "text"
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

	// MIMEType identifies the MIME type of the original content. Empty
	// means the provider did not surface it.
	MIMEType string `json:"mime_type,omitempty"`

	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific result metadata into Extra.
func (m *ResultMetadata) Set(key string, value any) error {
	if m == nil {
		return errors.New("embedding.ResultMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
}

// Result is one embedding plus its metadata.
type Result struct {
	// Embedding is the vector representation of the input.
	Embedding []float64 `json:"embedding"`

	// Metadata carries the source position, modality, and any extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when the embedding is
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

// Usage records the token consumption an embedding request reported back.
// Embedding is input-only — there is no completion, reasoning, or cache
// dimension — so a single count is the whole story. Providers that report
// a "total" figure map it here: for embeddings every token is input.
type Usage struct {
	// InputTokens are tokens consumed embedding the inputs.
	InputTokens int64 `json:"input_tokens"`
}

// ResponseMetadata holds response-level metadata: the model actually
// used, token usage, creation time, and provider extras.
type ResponseMetadata struct {
	// Model is the model name actually served.
	Model string `json:"model"`

	// Usage breaks down token consumption. nil means the provider did not
	// report usage.
	Usage *Usage `json:"usage,omitempty"`

	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific response metadata into Extra.
func (m *ResponseMetadata) Set(key string, value any) error {
	if m == nil {
		return errors.New("embedding.ResponseMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
}

// Response is the full embedding result: one [*Result] per input plus
// shared response metadata.
type Response struct {
	// Results holds one entry per input text, in the same order.
	Results []*Result `json:"results,omitzero"`

	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from at least one result and a
// non-nil metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("embedding.NewResponse: at least one Result is required")
	}
	if metadata == nil {
		return nil, errors.New("embedding.NewResponse: metadata must not be nil")
	}
	return &Response{Results: results, Metadata: metadata}, nil
}

// First returns the first embedding — the common single-input shortcut.
// Returns nil when Results is empty.
func (r *Response) First() *Result {
	if r == nil || len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
