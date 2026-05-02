package chat

import (
	"errors"

	"github.com/Tangerg/lynx/core/model"
)

// FinishReason explains why an LLM stopped generating tokens. Providers
// vary on the exact spellings; the constants below cover the common cases
// and providers that emit a different value should map onto these or use
// [FinishReasonOther].
type FinishReason string

func (r FinishReason) String() string { return string(r) }

const (
	// FinishReasonStop — natural completion or a stop sequence was reached.
	FinishReasonStop FinishReason = "stop"

	// FinishReasonLength — generation hit the max-tokens limit.
	FinishReasonLength FinishReason = "length"

	// FinishReasonToolCalls — generation paused so the caller can execute tool calls.
	FinishReasonToolCalls FinishReason = "tool_calls"

	// FinishReasonContentFilter — provider-side safety filter blocked the response.
	FinishReasonContentFilter FinishReason = "content_filter"

	// FinishReasonReturnDirect — internal short-circuit: a tool result is
	// being returned directly without re-prompting the LLM.
	FinishReasonReturnDirect FinishReason = "return_direct"

	// FinishReasonOther — provider-specific reason that doesn't fit the
	// above; inspect [ResultMetadata.Extra] for details.
	FinishReasonOther FinishReason = "other"

	// FinishReasonNull — completion finished but the provider did not
	// surface a reason. Treat as "stop" unless your code differentiates.
	FinishReasonNull FinishReason = "null"
)

// ResultMetadata holds completion-level metadata for a single [Result] —
// the finish reason plus any provider-specific keys.
type ResultMetadata struct {
	// FinishReason explains why this generation stopped.
	FinishReason FinishReason `json:"finish_reason"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra"`
}

// ensureExtra lazily allocates Extra.
func (m *ResultMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag.
func (m *ResultMetadata) Get(key string) (any, bool) {
	m.ensureExtra()
	value, exists := m.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (m *ResultMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Result is one generation: the assistant's reply, completion metadata,
// and (when the assistant called tools) the tool execution results.
type Result struct {
	// AssistantMessage is the model's reply.
	AssistantMessage *AssistantMessage `json:"assistant_message"`

	// Metadata carries the finish reason and any per-result extras.
	Metadata *ResultMetadata `json:"metadata"`

	// ToolMessage carries tool execution results when the assistant invoked
	// tools. nil means no tool calls were made.
	ToolMessage *ToolMessage `json:"tool_message"`
}

// NewResult builds a [Result] from a non-nil assistant message and
// metadata. Tool results, if any, are attached separately by the caller.
func NewResult(assistantMessage *AssistantMessage, metadata *ResultMetadata) (*Result, error) {
	if assistantMessage == nil {
		return nil, errors.New("chat.NewResult: assistant message must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("chat.NewResult: metadata must not be nil")
	}

	return &Result{
		AssistantMessage: assistantMessage,
		Metadata:         metadata,
	}, nil
}

// Usage is an alias for [model.Usage] kept here so chat-package callers
// can use chat.Usage without an explicit /core/model import.
type Usage = model.Usage

// RateLimit is an alias for [model.RateLimit].
type RateLimit = model.RateLimit

// ResponseMetadata holds response-level metadata returned by the provider:
// id, model name actually served, token usage, rate-limit state, and any
// provider-specific extras.
type ResponseMetadata struct {
	// ID is the provider-assigned response id.
	ID string `json:"id"`

	// Model is the model name actually served (may differ from the
	// requested model id when the provider routed to a fallback).
	Model string `json:"model"`

	// Usage breaks down token consumption.
	Usage *Usage `json:"usage"`

	// RateLimit reports quota state at request time.
	RateLimit *RateLimit `json:"rate_limit"`

	// Created is the provider-reported response creation time, expressed
	// as Unix seconds.
	Created int64 `json:"created"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra"`
}

func (m *ResponseMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag.
func (m *ResponseMetadata) Get(key string) (any, bool) {
	m.ensureExtra()
	value, exists := m.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
func (m *ResponseMetadata) Set(key string, value any) {
	m.ensureExtra()
	m.Extra[key] = value
}

// Response is the full chat completion result: every generated alternative
// (typically one) plus shared response metadata.
type Response struct {
	// Results holds one entry per generated alternative.
	Results []*Result `json:"results"`

	// Metadata carries shared response-level fields (id, model, usage, ...).
	Metadata *ResponseMetadata `json:"metadata"`
}

// NewResponse builds a [Response] from at least one result and a non-nil
// metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	if len(results) == 0 {
		return nil, errors.New("chat.NewResponse: at least one Result is required")
	}
	if metadata == nil {
		return nil, errors.New("chat.NewResponse: metadata must not be nil")
	}

	return &Response{
		Results:  results,
		Metadata: metadata,
	}, nil
}

// Result returns the first generation alternative — the common
// "give me the answer" shortcut. Returns nil when Results is empty.
func (r *Response) Result() *Result {
	if len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}

// findFirstResultWithToolCalls returns the first Result whose assistant
// message issued tool calls, or nil if none did.
func (r *Response) findFirstResultWithToolCalls() *Result {
	for _, result := range r.Results {
		if result.AssistantMessage.HasToolCalls() {
			return result
		}
	}
	return nil
}
