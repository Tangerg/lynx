package transcription

import "errors"

// ResultMetadata holds per-segment metadata returned by the provider.
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

// Result is one transcription segment.
type Result struct {
	// Text is the transcribed text. Empty is allowed for partial /
	// silence segments.
	Text string `json:"text"`

	// Metadata carries per-segment extras.
	Metadata *ResultMetadata `json:"metadata"`
}

// NewResult builds a [Result]. Text may be empty; metadata is required.
func NewResult(text string, metadata *ResultMetadata) (*Result, error) {
	if metadata == nil {
		return nil, errors.New("transcription.NewResult: metadata must not be nil")
	}
	return &Result{Text: text, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata for a transcription call.
type ResponseMetadata struct {
	// Model is the model name actually served.
	Model string `json:"model"`

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

// Response is the full transcription result: every segment the
// provider produced plus shared response metadata.
type Response struct {
	// Results holds one entry per transcribed segment.
	Results []*Result `json:"results"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse builds a [Response] from at least one result and a
// non-nil metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("transcription.NewResponse: at least one Result is required")
	}
	if metadata == nil {
		return nil, errors.New("transcription.NewResponse: metadata must not be nil")
	}
	return &Response{Results: results, Metadata: metadata}, nil
}

// Result returns the first transcription segment — convenient when the
// provider produced one continuous transcription. Returns nil when
// Results is empty.
func (r *Response) Result() *Result {
	if len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
