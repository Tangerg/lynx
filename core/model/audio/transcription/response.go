package transcription

import "errors"

// ResultMetadata holds per-segment metadata returned by the provider.
type ResultMetadata struct {
	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

func (m *ResultMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (m *ResultMetadata) Get(key string) (any, bool) {
	if m == nil || m.Extra == nil {
		return nil, false
	}
	value, exists := m.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (m *ResultMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Result is one transcription segment.
type Result struct {
	// Text is the transcribed text. Empty is allowed for partial /
	// silence segments.
	Text string `json:"text"`

	// Metadata carries per-segment extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
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
	Extra map[string]any `json:"extra,omitzero"`
}

func (m *ResponseMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (m *ResponseMetadata) Get(key string) (any, bool) {
	if m == nil || m.Extra == nil {
		return nil, false
	}
	value, exists := m.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (m *ResponseMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Response is one transcription call's output plus shared metadata.
// Providers that emit per-segment timing (Whisper verbose_json) should
// stash the segment array under Result.Metadata.Extra; the top-level
// Result holds the merged transcript text.
type Response struct {
	// Result holds the transcribed text. Non-nil after [NewResponse].
	Result *Result `json:"result,omitempty"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from a non-nil result and metadata.
func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, errors.New("transcription.NewResponse: result must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("transcription.NewResponse: metadata must not be nil")
	}
	return &Response{Result: result, Metadata: metadata}, nil
}
