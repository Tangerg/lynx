package image

import "errors"

// Image holds one generated image, either as a URL pointing at hosted
// bytes or as a base64-encoded inline payload (mutually exclusive in
// practice, depending on the provider's chosen response format).
type Image struct {
	// URL is the hosted image URL. Empty when the provider returned bytes.
	URL string `json:"url"`

	// B64JSON is the base64-encoded image bytes. Empty when URL is set.
	B64JSON string `json:"b64_json"`
}

// At least one
// must be supplied — both empty returns an error.
func NewImage(url, b64JSON string) (*Image, error) {
	if url == "" && b64JSON == "" {
		return nil, errors.New("image.NewImage: at least one of URL or B64JSON is required")
	}
	return &Image{URL: url, B64JSON: b64JSON}, nil
}

// ResultMetadata holds per-image metadata returned by the provider.
type ResultMetadata struct {
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

// Result is one generated image plus its metadata.
type Result struct {
	// Image holds the generated image payload.
	Image *Image `json:"image,omitempty"`

	// Metadata carries per-image extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// Returns an error when image or metadata is nil.
func NewResult(image *Image, metadata *ResultMetadata) (*Result, error) {
	if image == nil {
		return nil, errors.New("image.NewResult: image must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("image.NewResult: metadata must not be nil")
	}
	return &Result{Image: image, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata for an image
// generation request.
type ResponseMetadata struct {
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

// Response is the full image-generation result: the rendered image plus
// shared response metadata.
//
// The image surface is one-image-per-call by design. Providers that accept
// `n` (OpenAI DALL-E 2) still return only the first image through this
// surface; callers needing N>1 should drop down to the provider's native
// SDK. See the rationale on chat.Response.
type Response struct {
	// Result is the generated image. Non-nil after [NewResponse].
	Result *Result `json:"result,omitempty"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, errors.New("image.NewResponse: result must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("image.NewResponse: metadata must not be nil")
	}
	return &Response{Result: result, Metadata: metadata}, nil
}
