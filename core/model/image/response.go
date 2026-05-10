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

// NewImage builds an [Image] from a URL or base64 payload. At least one
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
	Extra map[string]any `json:"extra"`
}

func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *ResultMetadata) Get(key string) (any, bool) {
	if r.Extra == nil {
		return nil, false
	}
	value, exists := r.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (r *ResultMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Result is one generated image plus its metadata.
type Result struct {
	// Image holds the generated image payload.
	Image *Image `json:"image"`

	// Metadata carries per-image extras.
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult builds a [Result]. Returns an error when image or metadata
// is nil.
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
	Extra map[string]any `json:"extra"`
}

func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *ResponseMetadata) Get(key string) (any, bool) {
	if r.Extra == nil {
		return nil, false
	}
	value, exists := r.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (r *ResponseMetadata) Set(key string, value any) {
	r.ensureExtra()
	r.Extra[key] = value
}

// Response is the full image-generation result: every alternative the
// provider rendered plus shared response metadata.
type Response struct {
	// Results holds one entry per generated alternative.
	Results []*Result `json:"results"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse builds a [Response] from at least one result and a
// non-nil metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("image.NewResponse: at least one Result is required")
	}
	if metadata == nil {
		return nil, errors.New("image.NewResponse: metadata must not be nil")
	}
	return &Response{Results: results, Metadata: metadata}, nil
}

// Result returns the first generation alternative — the common
// "give me the image" shortcut. Returns nil when Results is empty.
func (r *Response) Result() *Result {
	if len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
