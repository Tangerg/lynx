package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/mime"
)

// ModalityType represents the type of content that can be embedded.
type ModalityType string

const (
	// Text represents textual content for embedding.
	Text ModalityType = "text"

	// Image represents image content for embedding.
	Image ModalityType = "image"

	// Audio represents audio content for embedding.
	Audio ModalityType = "audio"

	// Video represents video content for embedding.
	Video ModalityType = "video"
)

func (m ModalityType) String() string {
	return string(m)
}

// ResultMetadata contains metadata information about an individual embedding result.
type ResultMetadata struct {
	// Index represents the position of this result in the original input list.
	Index int64 `json:"index"`

	// ModalityType indicates the type of content that was embedded (text, image, audio, or video).
	ModalityType ModalityType `json:"modality_type"`

	// MimeType specifies the MIME type of the original content, if applicable.
	MimeType *mime.MIME `json:"mime_type"`

	// Extra holds provider-specific metadata that is not part of the standard fields.
	Extra map[string]any `json:"extra"`
}

func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	value, exists := r.Extra[key]
	return value, exists
}

func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Result represents a single embedding vector along with its associated metadata.
type Result struct {
	// Embedding contains the vector representation as a slice of floating-point numbers.
	Embedding []float64 `json:"embedding"`

	// Metadata provides additional information about this embedding result.
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult creates a new embedding result with the given embedding vector and metadata.
// Returns an error if the embedding is empty or if metadata is nil.
func NewResult(embedding []float64, metadata *ResultMetadata) (*Result, error) {
	if len(embedding) == 0 {
		return nil, errors.New("embedding cannot be empty")
	}

	if metadata == nil {
		return nil, errors.New("metadata cannot be nil")
	}

	return &Result{
		Embedding: embedding,
		Metadata:  metadata,
	}, nil
}

// ResponseMetadata contains metadata information about the entire embedding response.
type ResponseMetadata struct {
	// Model identifies which embedding model was used to generate the embeddings.
	Model string `json:"model"`

	// Usage contains token usage statistics for the embedding request.
	Usage *chat.Usage `json:"usage"`

	// RateLimit provides information about API rate limit status.
	RateLimit *chat.RateLimit `json:"rate_limit"`

	// Created is the Unix timestamp indicating when the embeddings were generated.
	Created int64 `json:"created"`

	// Extra holds provider-specific metadata that is not part of the standard fields.
	Extra map[string]any `json:"extra"`
}

func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

func (r *ResponseMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	value, exists := r.Extra[key]
	return value, exists
}

func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Response represents the complete response from an embedding request,
// containing multiple embedding results and response-level metadata.
type Response struct {
	// Results contains all the embedding vectors generated from the input texts.
	// Each result corresponds to one input item in the original request.
	Results []*Result `json:"results"`

	// Metadata provides information about the embedding generation process.
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse creates a new embedding response with the given results and metadata.
// Returns an error if results is empty or if metadata is nil.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("results cannot be empty: at least one result is required")
	}

	if metadata == nil {
		return nil, errors.New("metadata cannot be nil")
	}

	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first embedding result from the response.
// This is a convenience method for single-input embedding requests.
// Returns nil if the response contains no results.
func (r *Response) Result() *Result {
	if len(r.Results) > 0 {
		return r.Results[0]
	}

	return nil
}
