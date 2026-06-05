package tts

import "errors"

// ResultMetadata holds per-segment metadata returned by the provider.
type ResultMetadata struct {
	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (r *ResultMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *ResultMetadata) Get(key string) (any, bool) {
	if r == nil || r.Extra == nil {
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

// Result is one chunk of generated audio. For synchronous calls the
// chunk is the entire audio; for streaming calls Speech is whatever
// segment the provider just produced.
type Result struct {
	// Speech holds the encoded audio bytes (encoding determined by
	// Request.Options.ResponseFormat).
	Speech []byte `json:"speech,omitzero"`

	// Metadata carries per-chunk extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when speech is empty
// or metadata is nil.
func NewResult(speech []byte, metadata *ResultMetadata) (*Result, error) {
	if len(speech) == 0 {
		return nil, errors.New("tts.NewResult: speech must not be empty")
	}
	if metadata == nil {
		return nil, errors.New("tts.NewResult: metadata must not be nil")
	}
	return &Result{Speech: speech, Metadata: metadata}, nil
}

// ResponseMetadata holds response-level metadata for a TTS call.
type ResponseMetadata struct {
	// Model is the model name actually served.
	Model string `json:"model"`

	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (r *ResponseMetadata) ensureExtra() {
	if r.Extra == nil {
		r.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *ResponseMetadata) Get(key string) (any, bool) {
	if r == nil || r.Extra == nil {
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

// Response is one TTS call's audio output plus shared metadata. For
// synchronous calls Result holds the entire audio; for streaming calls
// each chunk yields a Response with the just-produced segment in Result.
type Response struct {
	// Result holds the generated audio. Non-nil after [NewResponse].
	Result *Result `json:"result,omitempty"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from a non-nil result and metadata.
func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, errors.New("tts.NewResponse: result must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("tts.NewResponse: metadata must not be nil")
	}
	return &Response{Result: result, Metadata: metadata}, nil
}
