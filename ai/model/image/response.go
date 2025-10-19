package image

import (
	"errors"
)

// Image represents a generated image, which can be provided either as a URL or base64-encoded JSON
type Image struct {
	// URL is the direct link to the generated image
	URL string `json:"url"`

	// B64Json is the base64-encoded representation of the image
	B64Json string `json:"b64_json"`
}

// NewImage creates a new Image instance
// At least one of URL or b64Json must be provided
// Returns an error if both parameters are empty
func NewImage(URL string, b64Json string) (*Image, error) {
	if URL == "" && b64Json == "" {
		return nil, errors.New("no URL or B64 JSON provided")
	}
	return &Image{
		URL:     URL,
		B64Json: b64Json,
	}, nil
}

// ResultMetadata holds metadata information for a single image generation result
type ResultMetadata struct {
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

// Result represents a single image generation result with its associated metadata
type Result struct {
	// Image contains the generated image data
	Image *Image `json:"image"`

	// Metadata contains additional information about the generation result
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult creates a new Result instance
// Both image and metadata are required parameters
// Returns an error if either parameter is nil
func NewResult(image *Image, metadata *ResultMetadata) (*Result, error) {
	if image == nil {
		return nil, errors.New("image cannot be nil")
	}
	if metadata == nil {
		return nil, errors.New("metadata cannot be nil")
	}
	return &Result{
		Image:    image,
		Metadata: metadata,
	}, nil
}

// ResponseMetadata holds metadata information for the entire image generation response
type ResponseMetadata struct {
	// Created is the Unix timestamp of response creation
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

// Response represents the complete response from an image generation request
// It contains one or more results and associated metadata
type Response struct {
	// Results contains all generated image results
	Results []*Result

	// Metadata contains information about the response itself
	Metadata *ResponseMetadata
}

// NewResponse creates a new Response instance
// At least one result is required, and metadata must be provided
// Returns an error if results is empty or metadata is nil
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("at least one result is required")
	}
	if metadata == nil {
		return nil, errors.New("metadata is required")
	}
	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first result from the Results slice
// Returns nil if the Results slice is empty
// This is a convenience method for accessing the primary result
func (r *Response) Result() *Result {
	if len(r.Results) > 0 {
		return r.Results[0]
	}

	return nil
}
