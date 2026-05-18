package chat

// PartKind tags every [OutputPart] so consumers can do type-switch
// style dispatch without reflection.
type PartKind string

const (
	// PartKindText is plain assistant-emitted text.
	PartKindText PartKind = "text"

	// PartKindReasoning is visible chain-of-thought from reasoning-style
	// providers (Anthropic thinking, OpenAI o-series, DeepSeek-R1,
	// Gemini thought parts).
	PartKindReasoning PartKind = "reasoning"

	// PartKindToolCall is a single tool-invocation request.
	PartKindToolCall PartKind = "tool_call"
)

// OutputPart is the sealed marker for one ordered chunk in an
// AssistantMessage. The v1 universe is closed: exactly three concrete
// types — [TextPart], [ReasoningPart], [ToolCallPart]. Future part
// kinds (media / source / file / approval / custom / tool-error) are
// deferred until real use cases land; the [Accumulator] is fully
// type-agnostic and does not need to change when new parts are added.
//
// Streaming semantics: providers emit one or more [OutputPart] deltas
// per chunk. Same-type adjacent deltas merge in-place via
// [OutputPart.appendDelta]. Type changes — or identity changes for
// [ToolCallPart] — flush the in-flight part and start a new one.
// Providers MUST emit non-interleaved delta sequences: same logical
// part's deltas arrive contiguously.
type OutputPart interface {
	// Kind returns the part type for switch-style dispatch.
	Kind() PartKind

	// appendDelta tries to merge delta into this part in place.
	// Returns true on a successful in-place merge; false when delta
	// belongs to a new logical part (different concrete type, or
	// different identity such as a new tool call ID).
	//
	// Unexported by design: doubles as the sealed-union mechanism so
	// only types defined in this package can satisfy OutputPart.
	appendDelta(delta OutputPart) bool
}

// TextPart is plain assistant-emitted text. Same-type deltas
// concatenate their Text fields and merge metadata last-write-wins.
type TextPart struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitzero"`
}

// Kind reports [PartKindText].
func (p *TextPart) Kind() PartKind { return PartKindText }

func (p *TextPart) appendDelta(d OutputPart) bool {
	o, ok := d.(*TextPart)
	if !ok {
		return false
	}
	p.Text += o.Text
	mergeMeta(&p.Metadata, o.Metadata)
	return true
}

// ReasoningPart carries visible chain-of-thought. Signature preserves
// the vendor-opaque continuation token (Anthropic thought_signature,
// Google thoughtSignatures, OpenAI encrypted reasoning) so the block
// can round-trip in a follow-up request.
//
// Anthropic redacted thinking is represented by leaving Text as the
// SDK-supplied placeholder and tagging Metadata["redacted"]=true; a
// dedicated part type can be added later if call sites prove the
// metadata approach is too thin.
type ReasoningPart struct {
	Text      string         `json:"text"`
	Signature []byte         `json:"signature,omitempty"`
	Metadata  map[string]any `json:"metadata,omitzero"`
}

// Kind reports [PartKindReasoning].
func (p *ReasoningPart) Kind() PartKind { return PartKindReasoning }

func (p *ReasoningPart) appendDelta(d OutputPart) bool {
	o, ok := d.(*ReasoningPart)
	if !ok {
		return false
	}
	p.Text += o.Text
	if len(o.Signature) > 0 {
		p.Signature = o.Signature
	}
	mergeMeta(&p.Metadata, o.Metadata)
	return true
}

// ToolCallState tracks the lifecycle of a single [ToolCallPart].
// Streaming state mirrors here so static snapshots also expose where
// a tool call is in its lifecycle. Approval-related states are
// deferred to a future revision along with the HITL flow.
type ToolCallState string

const (
	// ToolCallStateInputStreaming — partial arguments JSON is arriving.
	ToolCallStateInputStreaming ToolCallState = "input_streaming"

	// ToolCallStateInputComplete — arguments JSON is fully assembled,
	// the runtime has not yet executed the tool.
	ToolCallStateInputComplete ToolCallState = "input_complete"

	// ToolCallStateExecuted — the runtime has invoked the tool; the
	// matching result/error lives in the following ToolMessage.
	ToolCallStateExecuted ToolCallState = "executed"
)

// rank returns a monotonic ordering for [ToolCallState] so accumulator
// merges can keep the highest state seen so far.
func (s ToolCallState) rank() int {
	switch s {
	case ToolCallStateInputStreaming:
		return 1
	case ToolCallStateInputComplete:
		return 2
	case ToolCallStateExecuted:
		return 3
	default:
		return 0
	}
}

// ToolCallPart is one tool invocation request. The ID flows through
// to the matching tool result in the following ToolMessage so callers
// can pair them by ID.
//
// Delta semantics: a delta with an empty ID is treated as a
// continuation of the in-flight part; a delta with a non-empty ID
// matching the in-flight part also continues it. A delta with a
// different non-empty ID is rejected — it belongs to a new tool call.
//
// This handles streaming protocols like OpenAI Chat Completions where
// multiple tool_calls grow in parallel — the provider adapter is
// responsible for buffering by vendor index and emitting each call as
// a contiguous run of deltas with the same ID.
type ToolCallPart struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments string         `json:"arguments,omitempty"` // JSON-encoded; grows with deltas
	State     ToolCallState  `json:"state,omitempty"`
	Metadata  map[string]any `json:"metadata,omitzero"`
}

// Kind reports [PartKindToolCall].
func (p *ToolCallPart) Kind() PartKind { return PartKindToolCall }

func (p *ToolCallPart) appendDelta(d OutputPart) bool {
	o, ok := d.(*ToolCallPart)
	if !ok {
		return false
	}
	if o.ID != "" && p.ID != "" && o.ID != p.ID {
		return false // different logical tool call
	}
	if p.ID == "" {
		p.ID = o.ID
	}
	if p.Name == "" {
		p.Name = o.Name
	}
	p.Arguments += o.Arguments
	if o.State.rank() > p.State.rank() {
		p.State = o.State
	}
	mergeMeta(&p.Metadata, o.Metadata)
	return true
}

// mergeMeta copies src entries into *dst, allocating *dst lazily. Used
// by every [OutputPart] implementation; last-write-wins for duplicate
// keys.
func mergeMeta(dst *map[string]any, src map[string]any) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		(*dst)[k] = v
	}
}
