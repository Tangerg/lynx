package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/pkg/ptr"
)

// Options holds the per-request configuration LLM providers accept.
// Standard parameters (model id, temperature, ...) are typed; anything a
// specific provider needs is stored in Extra. Pointer-typed fields use nil
// to mean "not set"; the provider applies its own default.
type Options struct {
	// Model is the provider model identifier (e.g. "gpt-4o", "claude-3-5-sonnet").
	Model string `json:"model"`

	// FrequencyPenalty discourages repeated tokens. Range -2.0 to 2.0.
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`

	// MaxTokens caps the number of tokens the model may generate.
	MaxTokens *int64 `json:"max_tokens,omitempty"`

	// PresencePenalty discourages already-mentioned tokens. Range -2.0 to 2.0.
	PresencePenalty *float64 `json:"presence_penalty,omitempty"`

	// Stop terminates generation when any sequence is produced.
	Stop []string `json:"stop,omitzero"`

	// Temperature controls sampling randomness. Range 0.0 to 2.0.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopK limits sampling to the K highest-probability tokens.
	TopK *int64 `json:"top_k,omitempty"`

	// TopP enables nucleus sampling using the cumulative probability mass.
	TopP *float64 `json:"top_p,omitempty"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error if
// model is empty — every provider requires a model id.
//
// Example:
//
//	opts, err := chat.NewOptions("gpt-4o")
//	if err != nil { return err }
//	opts.Set("response_format", map[string]any{"type": "json_object"})
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("chat.NewOptions: model id must not be empty")
	}

	return &Options{
		Model: model,
	}, nil
}

// ensureExtra lazily allocates Extra. Used by [Options.Set] only —
// reads must NOT mutate state since Get can be invoked concurrently.
func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. Safe to
// call concurrently with other Get calls; concurrent with Set is not.
func (o *Options) Get(key string) (any, bool) {
	if o == nil || o.Extra == nil {
		return nil, false
	}
	value, exists := o.Extra[key]
	return value, exists
}

// Set stores value under key in Extra, allocating the map if needed.
func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

// Clone returns a deep copy of Options. Pointer fields, slices, and the
// Extra map are duplicated so the result is safe to mutate independently.
// A nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}

	return &Options{
		Model:            o.Model,
		FrequencyPenalty: ptr.Clone(o.FrequencyPenalty),
		MaxTokens:        ptr.Clone(o.MaxTokens),
		PresencePenalty:  ptr.Clone(o.PresencePenalty),
		Stop:             slices.Clone(o.Stop),
		Temperature:      ptr.Clone(o.Temperature),
		TopK:             ptr.Clone(o.TopK),
		TopP:             ptr.Clone(o.TopP),
		Extra:            maps.Clone(o.Extra),
	}
}

// MergeOptions clones base then applies each override left-to-right.
// Scalar non-empty values overwrite; slices append; the Extra map merges
// last-write-wins.
//
// Returns an error when base is nil so callers don't accidentally produce
// a Options without a model id.
//
// Example:
//
//	merged, err := chat.MergeOptions(modelDefaults, requestOverrides)
//	if err != nil { return err }
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("chat.MergeOptions: base options must not be nil")
	}

	merged := ptr.Clone(base)
	if len(overrides) == 0 {
		return merged, nil
	}

	merged.ensureExtra()

	for _, override := range overrides {
		if override == nil {
			continue
		}
		merged.applyOverride(override)
	}
	return merged, nil
}

// applyOverride mutates the receiver in place with the non-zero fields of
// src. Extracted from MergeOptions to keep the merge body free of repeated
// "if-not-zero overwrite" boilerplate.
func (o *Options) applyOverride(src *Options) {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.FrequencyPenalty != nil {
		o.FrequencyPenalty = src.FrequencyPenalty
	}
	if src.MaxTokens != nil {
		o.MaxTokens = src.MaxTokens
	}
	if src.PresencePenalty != nil {
		o.PresencePenalty = src.PresencePenalty
	}
	if len(src.Stop) > 0 {
		// Replace, not append: every other scalar field overrides on
		// non-zero, and appending makes MergeOptions non-idempotent —
		// merging the same override N times would multiply stop
		// sequences. Clone so callers can mutate either slice safely.
		o.Stop = slices.Clone(src.Stop)
	}
	if src.Temperature != nil {
		o.Temperature = src.Temperature
	}
	if src.TopK != nil {
		o.TopK = src.TopK
	}
	if src.TopP != nil {
		o.TopP = src.TopP
	}
	if len(src.Extra) > 0 {
		maps.Copy(o.Extra, src.Extra)
	}
}

// Request is a chat completion request: the conversation history, the
// tools the model may invoke, the model options, and free-form
// per-request parameters (user id, session id, etc. — whatever
// middleware wants to thread through).
type Request struct {
	// Messages is the conversation history sent to the model.
	Messages []Message `json:"messages,omitzero"`

	// Tools the model may invoke during generation. Excluded from JSON
	// to keep the wire format provider-agnostic — serialization is the
	// provider's job. Sits at the Request level (not inside Options)
	// because tools are capability, not sampling configuration.
	Tools []Tool `json:"-"`

	// Options carries model-specific parameters.
	Options *Options `json:"options,omitempty"`

	// Params holds caller-supplied side-channel data (user id, trace id,
	// feature flags) that middlewares can read.
	Params map[string]any `json:"params,omitzero"`
}

// ConversationIDKey is the [Request.Params] key identifying the
// conversation a request belongs to — a protocol-level convention (not
// any single middleware's secret) so the layer that owns the conversation
// stamps it once and every middleware that cares reads it from the request:
//
//   - the memory middleware keys stored history by it,
//   - the tool middleware keys parked (interrupted) rounds by it.
//
// It lives here, on the protocol type, rather than inside a middleware
// package so the producer (e.g. the agent runtime, which knows the
// conversation/session id per process) can stamp it without importing any
// middleware implementation. Set it before the call:
//
//	req.Set(chat.ConversationIDKey, "session-42")
//
// Absent, the memory middleware passes through (no load/save) and the tool
// middleware skips park persistence.
const ConversationIDKey = "lynx:ai:model:chat:conversation_id"

// NewRequest builds a Request from the given messages. nil entries are
// filtered out; an empty (or nil-only) input returns an error.
//
// Example:
//
//	req, err := chat.NewRequest([]chat.Message{
//	    chat.NewSystemMessage("You are concise."),
//	    chat.NewUserMessage("Hello"),
//	})
func NewRequest(messages []Message) (*Request, error) {
	valid := filterOutNilMessages(messages)
	if len(valid) == 0 {
		return nil, errors.New("chat.NewRequest: messages must contain at least one non-nil entry")
	}

	return &Request{
		Messages: valid,
		Params:   make(map[string]any),
	}, nil
}

// ensureParams lazily allocates Params. Used by [Request.Set] only —
// reads must NOT mutate state since Get can be invoked concurrently.
func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

// ConversationID returns the conversation id stamped under
// [ConversationIDKey], or "" when the producer did not stamp one.
// A present but non-string value is a stamping bug: every consumer
// (memory history, tool park persistence) keys durable state by this
// id, so it surfaces as an error here — once, with one semantic —
// rather than each middleware independently deciding whether to error
// or silently treat it as absent.
func (r *Request) ConversationID() (string, error) {
	raw, exists := r.Get(ConversationIDKey)
	if !exists {
		return "", nil
	}
	id, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("chat: ConversationIDKey value must be a string, got %T", raw)
	}
	return id, nil
}

// Get returns the Params value for key plus an existence flag. Safe
// to call concurrently with other Get calls; concurrent with Set is not.
func (r *Request) Get(key string) (any, bool) {
	if r == nil || r.Params == nil {
		return nil, false
	}
	value, exists := r.Params[key]
	return value, exists
}

// Set stores value under key in Params.
func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}

// AppendToLastUserMessage appends text to the last user message, separated
// from existing content by a double newline. No-op when no user message
// exists.
func (r *Request) AppendToLastUserMessage(text string) {
	appendTextToLastMessageOfType(r.Messages, MessageTypeUser, text)
}

// ReplaceTextOfLastUserMessage replaces the entire text of the last
// user message. No-op when no user message exists.
func (r *Request) ReplaceTextOfLastUserMessage(text string) {
	replaceTextOfLastMessageOfType(r.Messages, MessageTypeUser, text)
}

// UserMessage returns the most-recent user message in the conversation,
// or an empty *UserMessage if the conversation has none.
func (r *Request) UserMessage() *UserMessage {
	if r == nil {
		return NewUserMessage("")
	}
	idx, last := findLastMessageIndexOfType(r.Messages, MessageTypeUser)
	if idx == -1 {
		return NewUserMessage("")
	}
	return last.(*UserMessage)
}

// SystemMessage returns the most-recent system message in the
// conversation, or an empty *SystemMessage if the conversation has none.
func (r *Request) SystemMessage() *SystemMessage {
	if r == nil {
		return NewSystemMessage("")
	}
	idx, last := findLastMessageIndexOfType(r.Messages, MessageTypeSystem)
	if idx == -1 {
		return NewSystemMessage("")
	}
	return last.(*SystemMessage)
}

// UnmarshalJSON decodes a Request, dispatching each entry in the
// "messages" array to the concrete [Message] type indicated by its
// "type" discriminator. The standard library cannot decode into a
// []Message interface slice by itself, so the polymorphism lives
// here rather than scattered across every caller. Tools intentionally
// don't round-trip — they hold runtime closures and are excluded from
// JSON by design.
func (r *Request) UnmarshalJSON(data []byte) error {
	var raw struct {
		Messages []json.RawMessage `json:"messages,omitzero"`
		Options  *Options          `json:"options,omitempty"`
		Params   map[string]any    `json:"params,omitzero"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Options = raw.Options
	r.Params = raw.Params
	if len(raw.Messages) == 0 {
		r.Messages = nil
		return nil
	}

	r.Messages = make([]Message, 0, len(raw.Messages))
	for i, m := range raw.Messages {
		msg, err := UnmarshalMessage(m)
		if err != nil {
			return fmt.Errorf("chat.Request.UnmarshalJSON: messages[%d]: %w", i, err)
		}
		r.Messages = append(r.Messages, msg)
	}
	return nil
}
