package maintenance

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
)

// The non-LLM rungs of the compaction ladder: before paying for an LLM summary,
// replace the two cheap, deterministic sources of bloat in the OLD portion of a
// conversation — oversized tool-call arguments and oversized tool-result bodies
// — with previews. If that alone brings the footprint under budget the summary
// is skipped entirely (see [Compactor.MaybeCompact]).
//
// This trim is LOSSY and NOT retrievable, unlike the fresh-result offload
// (agentexec tool-result eviction, which keeps a read_tool_result handle): these
// are OLD messages already near the summary boundary, about to be folded into a
// summary on the next rung anyway, so a durable blob per body isn't warranted.
// The markers say so, and never point at read_tool_result, so the model won't
// try to retrieve them.

const (
	// ladderArgCap bounds a single tool-call argument blob. Tool arguments are
	// rarely large; a giant one (an inline file-write payload) is the exception
	// this catches. The replacement is valid JSON so a strict provider still
	// accepts the historical call on replay.
	ladderArgCap = 2000
	// ladderResultCap bounds a single OLD tool-result body kept in history.
	ladderResultCap = 2000
)

// trimForBudget replaces oversized tool-call arguments and tool-result bodies in
// the messages OLDER than the keep-recent window with previews, returning a
// COPY (never mutating the input's shared parts) and whether anything changed.
// The recent window is left untouched — the model most likely still needs its
// full detail. Copy-on-write: unchanged messages are shared, so a no-op trim
// allocates nothing.
func (c *Compactor) trimForBudget(msgs []chat.Message) ([]chat.Message, bool) {
	boundary := len(msgs) - c.keepRecent
	if boundary <= 0 {
		return msgs, false
	}
	out := msgs
	changed := false
	for i := range boundary {
		trimmed, ok := trimMessage(msgs[i])
		if !ok {
			continue
		}
		if !changed {
			out = slices.Clone(msgs)
			changed = true
		}
		out[i] = trimmed
	}
	return out, changed
}

// trimMessage returns a copy of m with each oversized tool-call/result part
// replaced by a preview, or (m, false) when nothing in it is oversized.
func trimMessage(m chat.Message) (chat.Message, bool) {
	parts := m.Parts
	changed := false
	for j := range m.Parts {
		trimmed, ok := trimPart(m.Parts[j])
		if !ok {
			continue
		}
		if !changed {
			parts = slices.Clone(m.Parts)
			changed = true
		}
		parts[j] = trimmed
	}
	if !changed {
		return m, false
	}
	return chat.Message{Role: m.Role, Parts: parts, Metadata: m.Metadata}, true
}

// trimPart previews an oversized tool-call argument blob or tool-result body,
// returning (clone, true) when it trimmed and (p, false) otherwise. The clone is
// independent (Part.Clone deep-copies the tool payload), so mutating it never
// touches the stored message.
func trimPart(p chat.Part) (chat.Part, bool) {
	switch {
	case p.Kind == chat.PartToolCall && p.ToolCall != nil && len(p.ToolCall.Arguments) > ladderArgCap:
		clone := p.Clone()
		clone.ToolCall.Arguments = fmt.Sprintf(`{"_trimmed":"%d bytes elided on compaction"}`, len(p.ToolCall.Arguments))
		return clone, true
	case p.Kind == chat.PartToolResult && p.ToolResult != nil && len(p.ToolResult.Result) > ladderResultCap:
		clone := p.Clone()
		clone.ToolResult.Result = clipResult(p.ToolResult.Result)
		return clone, true
	default:
		return p, false
	}
}

// clipResult previews an old tool-result body head+tail. The marker is
// deliberately distinct from the eviction placeholder and names no retrieval
// handle — this trim is lossy, so the model must not try to read it back.
func clipResult(s string) string {
	out, _ := headTail(s, ladderResultCap, func(elided int) string {
		return fmt.Sprintf("\n…[%d bytes trimmed on compaction; not retrievable]…\n", elided)
	})
	return out
}
