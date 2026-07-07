package chat

import (
	"iter"
	"slices"
	"strings"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// AssistantMessage is one model reply, carried as an ordered list of
// [OutputPart]s. Text, reasoning, and tool calls live in [Parts] in
// the order the model emitted them — text↔tool_use interleaving from
// Claude / Gemini / OpenAI Responses API is preserved verbatim.
//
// Helper accessors ([AssistantMessage.JoinedText],
// [AssistantMessage.JoinedReasoning], [AssistantMessage.ToolCalls],
// ...) derive flat views from Parts for code that just wants the
// final string or tool-call list.
type AssistantMessage struct {
	Parts    []OutputPart   `json:"parts"`
	Metadata map[string]any `json:"metadata,omitzero"`
}

func (a *AssistantMessage) message() {}

func (a *AssistantMessage) Type() MessageType { return MessageTypeAssistant }

// Meta returns the metadata map, allocating it on first access.
func (a *AssistantMessage) Meta() map[string]any {
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	return a.Metadata
}

func (a *AssistantMessage) TextParts() iter.Seq[*TextPart] {
	return partsOf[*TextPart](a)
}

func (a *AssistantMessage) ReasoningParts() iter.Seq[*ReasoningPart] {
	return partsOf[*ReasoningPart](a)
}

func (a *AssistantMessage) ToolCalls() iter.Seq[*ToolCallPart] {
	return partsOf[*ToolCallPart](a)
}

func partsOf[T OutputPart](a *AssistantMessage) iter.Seq[T] {
	return func(yield func(T) bool) {
		if a == nil {
			return
		}
		for _, p := range a.Parts {
			if tp, ok := p.(T); ok && !yield(tp) {
				return
			}
		}
	}
}

// CollectToolCalls returns the [ToolCallPart]s as a slice — the
// allocating counterpart of [AssistantMessage.ToolCalls] for sites
// that need indexed access or len().
func (a *AssistantMessage) CollectToolCalls() []*ToolCallPart {
	return slices.Collect(a.ToolCalls())
}

func (a *AssistantMessage) JoinedText() string {
	return joinTexts(a.TextParts(), func(p *TextPart) string { return p.Text })
}

func (a *AssistantMessage) JoinedReasoning() string {
	return joinTexts(a.ReasoningParts(), func(p *ReasoningPart) string { return p.Text })
}

func joinTexts[T any](seq iter.Seq[T], getText func(T) string) string {
	var b strings.Builder
	for p := range seq {
		b.WriteString(getText(p))
	}
	return b.String()
}

func (a *AssistantMessage) HasToolCalls() bool {
	if a == nil {
		return false
	}
	return slices.ContainsFunc(a.Parts, func(p OutputPart) bool {
		_, ok := p.(*ToolCallPart)
		return ok
	})
}

func (a *AssistantMessage) HasReasoning() bool {
	if a == nil {
		return false
	}
	return slices.ContainsFunc(a.Parts, func(p OutputPart) bool {
		rp, ok := p.(*ReasoningPart)
		return ok && rp.Text != ""
	})
}

// IsBlank reports whether this assistant message carries neither tool
// calls nor non-whitespace text — a round boundary with nothing to persist
// (history middleware) or re-prompt (tool middleware). A nil receiver
// returns true (the message is absent, so it has no content).
func (a *AssistantMessage) IsBlank() bool {
	if a == nil {
		return true
	}
	if a.HasToolCalls() {
		return false
	}
	return strings.TrimSpace(a.JoinedText()) == ""
}

// NewAssistantMessage builds an [AssistantMessage] from one of the
// supported parameter shapes — the type-set on T documents the
// accepted forms:
//
//   - string                → single [TextPart]
//   - []OutputPart          → use Parts verbatim
//   - []*ToolCallPart       → use as Parts (one per call)
//   - map[string]any        → metadata only (empty Parts)
//   - [MessageParams]       → full control
func NewAssistantMessage[T string | []OutputPart | []*ToolCallPart | map[string]any | MessageParams](param T) *AssistantMessage {
	params := paramsFromAssistantInput(param)

	if params.Metadata == nil {
		params.Metadata = make(map[string]any)
	}

	parts := params.Parts
	// MessageParams.Text — when supplied alongside Parts, gets emitted
	// as a trailing TextPart. When the only input is a string, the
	// switch in paramsFromAssistantInput already set Parts directly.
	if params.Text != "" && !textAlreadyInParts(parts, params.Text) {
		parts = append(parts, &TextPart{Text: params.Text})
	}

	return &AssistantMessage{
		Parts:    parts,
		Metadata: params.Metadata,
	}
}

func paramsFromAssistantInput[T string | []OutputPart | []*ToolCallPart | map[string]any | MessageParams](param T) MessageParams {
	var out MessageParams
	switch typed := any(param).(type) {
	case string:
		if typed != "" {
			out.Parts = []OutputPart{&TextPart{Text: typed}}
		}
	case []OutputPart:
		out.Parts = typed
	case []*ToolCallPart:
		out.Parts = pkgSlices.Map(typed, func(tc *ToolCallPart) OutputPart { return tc })
	case map[string]any:
		out.Metadata = typed
	case MessageParams:
		out = typed
	}
	return out
}

// textAlreadyInParts guards against double-appending Text when both
// Text and Parts are passed via MessageParams and Parts ends with the
// same string.
func textAlreadyInParts(parts []OutputPart, text string) bool {
	last, ok := pkgSlices.Last(parts)
	if !ok {
		return false
	}
	tp, isText := last.(*TextPart)
	return isText && tp.Text == text
}
