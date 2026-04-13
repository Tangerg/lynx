package tts

import (
	"errors"
)

// ResultMetadata holds metadata information for a single text-to-speech result
type ResultMetadata struct {
	// Extra holds provider-specific metadata that is not part of the standard fields
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

// Result represents a single text-to-speech generation result with its associated metadata
type Result struct {
	// Speech contains the generated audio data in binary format
	Speech []byte `json:"speech"`

	// Metadata contains additional information about the speech generation result
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult creates a new Result instance
// Both speech and metadata are required parameters
// Returns an error if speech is empty or metadata is nil
func NewResult(speech []byte, metadata *ResultMetadata) (*Result, error) {
	if len(speech) == 0 {
		return nil, errors.New("speech is empty")
	}
	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	return &Result{
		Speech:   speech,
		Metadata: metadata,
	}, nil
}

// ResponseMetadata holds metadata information for the entire text-to-speech response
type ResponseMetadata struct {
	// Model is the name of the TTS model used for generation
	Model string `json:"model"`

	// Created is the Unix timestamp of response creation
	Created int64 `json:"created"`

	// Extra holds provider-specific metadata that is not part of the standard fields
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

// Response represents the complete response from a text-to-speech generation request
// It contains one or more speech results and associated metadata
type Response struct {
	// Results contains all generated speech results
	Results []*Result `json:"results"`

	// Metadata contains information about the response itself
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse creates a new Response instance
// At least one result is required, and metadata must be provided
// Returns an error if results is empty or metadata is nil
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("at least one result is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata cannot be nil")
	}
	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first result from the Results slice
// Returns nil if the Results slice is empty
// This is a convenience method for accessing the primary speech result
func (r *Response) Result() *Result {
	if len(r.Results) > 0 {
		return r.Results[0]
	}
	return nil
}
