package chat

import (
	"encoding/json"
	"fmt"
	"slices"
)

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
// deferred until real use cases land; the part-level accumulator
// inside [ResponseAccumulator] is fully type-agnostic and does not
// need to change when new parts are added.
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

	// clone returns a deep copy. The part-level accumulator adopts a
	// clone (never the input delta itself) when starting a new running
	// part, so its later in-place appendDelta merges never mutate the
	// caller's chunk. This matters when the same chunk stream feeds more
	// than one accumulator (e.g. the tool-loop and memory stream
	// middlewares both accumulating): without the clone, the inner
	// accumulator's merge would mutate a part the outer one still holds,
	// double-counting the delta. Unexported — part of the sealed union.
	clone() OutputPart
}

// TextPart is plain assistant-emitted text. Same-type deltas
// concatenate their Text fields.
type TextPart struct {
	Text string `json:"text"`
}

// Kind reports [PartKindText].
func (p *TextPart) Kind() PartKind { return PartKindText }

func (p *TextPart) appendDelta(d OutputPart) bool {
	o, ok := d.(*TextPart)
	if !ok {
		return false
	}
	p.Text += o.Text
	return true
}

func (p *TextPart) clone() OutputPart { return &TextPart{Text: p.Text} }

// ReasoningPart carries visible chain-of-thought. Signature preserves
// the vendor-opaque continuation token (Anthropic thought_signature,
// Google thoughtSignatures, OpenAI encrypted reasoning) so the block
// can round-trip in a follow-up request.
//
// Anthropic redacted thinking is carried on the message-level
// [AssistantMessage.Metadata] (see anthropic.MetaRedactedReasoning),
// not on the part itself.
type ReasoningPart struct {
	Text      string `json:"text"`
	Signature []byte `json:"signature,omitempty"`
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
	return true
}

func (p *ReasoningPart) clone() OutputPart {
	return &ReasoningPart{Text: p.Text, Signature: slices.Clone(p.Signature)}
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"` // JSON-encoded; grows with deltas
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
	return true
}

func (p *ToolCallPart) clone() OutputPart {
	return &ToolCallPart{ID: p.ID, Name: p.Name, Arguments: p.Arguments}
}

// marshalOutputPart renders an [OutputPart] as a kind-tagged JSON
// object. Each part is encoded inline (no nested envelope) with a
// leading "kind" discriminator so the message-level JSON stays flat
// and decoders can dispatch in one pass.
func marshalOutputPart(p OutputPart) ([]byte, error) {
	switch tp := p.(type) {
	case *TextPart:
		return marshalKindedPart(PartKindText, tp)
	case *ReasoningPart:
		return marshalKindedPart(PartKindReasoning, tp)
	case *ToolCallPart:
		return marshalKindedPart(PartKindToolCall, tp)
	default:
		return nil, fmt.Errorf("chat: marshalOutputPart: unknown part type %T", p)
	}
}

// marshalKindedPart marshals val and splices a leading "kind" field
// into the resulting JSON object. Avoids a second pass through
// map[string]any.
func marshalKindedPart[T any](kind PartKind, val T) ([]byte, error) {
	body, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("chat: marshalKindedPart: %s did not encode as an object", kind)
	}
	prefix := `{"kind":"` + string(kind) + `"`
	if len(body) == 2 { // body == "{}"
		return []byte(prefix + "}"), nil
	}
	return []byte(prefix + "," + string(body[1:])), nil
}

// unmarshalOutputPart decodes a kind-tagged JSON object back into the
// matching concrete [OutputPart] implementation.
func unmarshalOutputPart(data []byte) (OutputPart, error) {
	var head struct {
		Kind PartKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	switch head.Kind {
	case PartKindText:
		var p TextPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return &p, nil
	case PartKindReasoning:
		var p ReasoningPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return &p, nil
	case PartKindToolCall:
		var p ToolCallPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return &p, nil
	default:
		return nil, fmt.Errorf("chat: unmarshalOutputPart: unknown kind %q", head.Kind)
	}
}
