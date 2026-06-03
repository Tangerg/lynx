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
	Extra map[string]any `json:"extra,omitzero"`
}

// ensureExtra lazily allocates Extra. Used by [ResultMetadata.Set]
// only — Get must not mutate state.
func (m *ResultMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. Safe
// to call concurrently with other Get calls; concurrent with Set is not.
func (m *ResultMetadata) Get(key string) (any, bool) {
	if m.Extra == nil {
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

// Result is one generation: the assistant's reply, completion metadata,
// and (when the assistant called tools) the tool execution results.
type Result struct {
	// AssistantMessage is the model's reply.
	AssistantMessage *AssistantMessage `json:"assistant_message,omitempty"`

	// Metadata carries the finish reason and any per-result extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`

	// ToolMessage carries tool execution results when the assistant invoked
	// tools. nil means no tool calls were made.
	ToolMessage *ToolMessage `json:"tool_message,omitempty"`
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
	Usage *Usage `json:"usage,omitempty"`

	// RateLimit reports quota state at request time.
	RateLimit *RateLimit `json:"rate_limit,omitempty"`

	// Created is the provider-reported response creation time, expressed
	// as Unix seconds.
	Created int64 `json:"created"`

	// Extra carries provider-specific metadata.
	Extra map[string]any `json:"extra,omitzero"`
}

// ensureExtra lazily allocates Extra. Used by [ResponseMetadata.Set]
// only — Get must not mutate state.
func (m *ResponseMetadata) ensureExtra() {
	if m.Extra == nil {
		m.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. Safe
// to call concurrently with other Get calls; concurrent with Set is not.
func (m *ResponseMetadata) Get(key string) (any, bool) {
	if m.Extra == nil {
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

// Response is the full chat completion result: the single generation
// plus shared response metadata.
//
// The chat surface is one-completion-per-call by design. Providers that
// accept an `n` / `candidateCount` parameter (OpenAI, Google) still return
// only the first choice through this surface; reach for the underlying SDK
// when multiple completions are actually needed (see mature frameworks like
// Spring AI / LangChain — `n` lives on provider-specific options, not the
// generic chat interface).
type Response struct {
	// Result is the assistant's generation. Non-nil after [NewResponse].
	Result *Result `json:"result,omitempty"`

	// Metadata carries shared response-level fields (id, model, usage, ...).
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from a non-nil result and metadata.
func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, errors.New("chat.NewResponse: result must not be nil")
	}
	if metadata == nil {
		return nil, errors.New("chat.NewResponse: metadata must not be nil")
	}

	return &Response{
		Result:   result,
		Metadata: metadata,
	}, nil
}

// TextDelta returns the assistant text this response chunk carries — the
// joined TextPart bodies of its assistant message. Returns "" when the
// chunk has no assistant text: a tool-call round (the assistant message
// holds only ToolCallParts), a tool-result round (see [Response.IsToolResult]),
// or a reasoning-only / empty chunk. Convenience for streaming consumers
// accumulating the visible reply across chunks; nil-safe on the receiver
// and the result/message chain.
func (r *Response) TextDelta() string {
	if r == nil || r.Result == nil || r.Result.AssistantMessage == nil {
		return ""
	}
	return r.Result.AssistantMessage.JoinedText()
}

// ReasoningDelta returns the extended-thinking text this response chunk
// carries — the joined ReasoningPart bodies of its assistant message.
// Returns "" for chunks without reasoning content. Mirrors [Response.TextDelta]
// but reads the reasoning subset, so consumers can surface thinking
// separately from the final reply.
func (r *Response) ReasoningDelta() string {
	if r == nil || r.Result == nil || r.Result.AssistantMessage == nil {
		return ""
	}
	return r.Result.AssistantMessage.JoinedReasoning()
}

// IsToolResult reports whether this is the synthetic tool-result chunk the
// tool-loop middleware (see [NewToolMiddleware]) yields between LLM rounds:
// Result.ToolMessage set, Result.AssistantMessage nil. It marks a round
// boundary — the prior LLM round is over and its usage is final — which
// streaming consumers use to commit per-round accounting before the next
// round begins.
func (r *Response) IsToolResult() bool {
	return r != nil &&
		r.Result != nil &&
		r.Result.AssistantMessage == nil &&
		r.Result.ToolMessage != nil
}
