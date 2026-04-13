package transcription

import (
	"errors"
)

// ResultMetadata holds metadata information for a single audio transcription result
type ResultMetadata struct {
	// Extra holds provider-specific metadata that is not part of the standard fields
	Extra map[string]any `json:"extra"`
}

// ensureExtra initializes the Extra map if it is nil
func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get retrieves a value from the Extra metadata map
// Returns the value and a boolean indicating whether the key exists
func (r *ResultMetadata) Get(key string) (any, bool) {
	r.ensureExtra()
	value, exists := r.Extra[key]
	return value, exists
}

// Set stores a key-value pair in the Extra metadata map
func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Result represents a single audio transcription result with its associated metadata
type Result struct {
	// Text contains the transcribed text from the audio
	Text string `json:"text"`

	// Metadata contains additional information about the transcription result
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult creates a new Result instance
// Metadata is required; text can be empty for partial or failed transcriptions
// Returns an error if metadata is nil
func NewResult(text string, metadata *ResultMetadata) (*Result, error) {
	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	return &Result{
		Text:     text,
		Metadata: metadata,
	}, nil
}

// ResponseMetadata holds metadata information for the entire audio transcription response
type ResponseMetadata struct {
	// Model is the name of the transcription model used
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

// Response represents the complete response from an audio transcription request
// It contains one or more transcription results and associated metadata
type Response struct {
	// Results contains all transcription results from the audio processing
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
// This is a convenience method for accessing the primary transcription result
func (r *Response) Result() *Result {
	if len(r.Results) > 0 {
		return r.Results[0]
	}
	return nil
}
