package server

import (
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// resumeBinding carries a parked run's pending toolCall item ids into its
// continuation translator. When a continuation resumes an approved tool, the
// tool re-fires and the translator reuses its ORIGINAL proposal item id instead
// of minting a fresh one — the item's runId is unchanged, since a resume
// continues the SAME run (there is no separate origin run to track).
type resumeBinding struct {
	toolItems map[string]string // resumeKey(toolName, arguments) -> original item id
	byName    map[string]string // toolName -> original item id; edited-args fallback ("" = ambiguous)
	questions []resumedQuestion // question items awaiting their terminal
}

// resumedQuestion is a question item from the interrupted run (ask_user /
// exit_plan_mode). Unlike a toolCall (which re-fires and completes on
// execution), a question is resolved by the resume answer itself — no event
// re-fires — so the continuation must emit its terminal item.completed
// explicitly (API.md §5.2) to close the proposal card. The Question payload
// is carried so the persisted completed item (items.list) keeps its content.
type resumedQuestion struct {
	itemID   string
	question *protocol.Question
}

// resumeBindingFrom extracts the pending approval items' ids (keyed by tool
// name + arguments) from a parked run so the continuation translator can
// reuse them when the approved tools re-fire. Returns nil when there are no
// approval interrupts (e.g. an ask_user / exit_plan_mode question, which
// resolves without a re-fired tool). The re-emitted items keep their original
// id + runId — a resume continues the SAME run, so the boundary is seamless.
func resumeBindingFrom(pending interrupts.Pending) *resumeBinding {
	var ints []protocol.Interrupt
	if err := json.Unmarshal(pending.Interrupts, &ints); err != nil || len(ints) == 0 {
		return nil
	}
	items := map[string]string{}
	byName := map[string]string{}
	// addItem indexes a proposal item both by (name, args) for an exact re-fire
	// match and by name alone for the edited-args fallback. A name shared by two
	// proposals is marked ambiguous ("") so the fallback won't guess.
	addItem := func(name, argsK, itemID string) {
		items[resumeKey(name, argsK)] = itemID
		if _, dup := byName[name]; dup {
			byName[name] = ""
		} else {
			byName[name] = itemID
		}
	}
	var questions []resumedQuestion
	for _, in := range ints {
		if in.ItemID == "" {
			continue
		}
		switch in.Type {
		case protocol.InterruptApproval:
			// Re-bind straight off payload.tool (API.md §4.8): the
			// domain-neutral ToolInvocation always carries name + arguments, so
			// the re-fired approved tool matches THIS proposal item by
			// (name, canonical arguments) — no backend-internal `_resume` tuple.
			tool, _ := in.Payload["tool"].(map[string]any)
			name, _ := tool["name"].(string)
			args, _ := tool["arguments"].(map[string]any)
			if name != "" {
				addItem(name, argsKey(args), in.ItemID)
			}
		case protocol.InterruptQuestion:
			// A question (ask_user / exit_plan_mode) is resolved by the resume
			// answer (no re-fired event), so the continuation must complete its item.
			questions = append(questions, resumedQuestion{itemID: in.ItemID, question: questionFromPayload(in.Payload)})
		}
	}
	// Tools that were still open at park time (e.g. the ask_user call
	// that interrupted from inside its own execution) re-fire on resume
	// and must reuse their ORIGINAL item ids — typed bookkeeping on the
	// pending record, never part of the wire payload.
	for _, dt := range pending.DrainedTools {
		if dt.Name == "" || dt.ItemID == "" {
			continue
		}
		addItem(dt.Name, argsKey(protocol.ParseArgs(dt.Arguments)), dt.ItemID)
	}
	if len(items) == 0 && len(questions) == 0 {
		return nil
	}
	return &resumeBinding{toolItems: items, byName: byName, questions: questions}
}

// questionFromPayload reconstructs the wire Question from an interrupt's
// payload map (round-tripped through JSON in the interrupt store) so the
// continuation's terminal item.completed carries the same content the
// proposal did. Returns nil when absent / malformed (the item still
// completes — just without re-stated content; the client already has it).
func questionFromPayload(payload map[string]any) *protocol.Question {
	raw, ok := payload["question"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var q protocol.Question
	if err := json.Unmarshal(b, &q); err != nil {
		return nil
	}
	return &q
}

// resumeKey identifies a gated tool call by (name, canonical-arguments) — the
// same pair the approval gate keys its verdict on, so a re-fired approved call
// matches the pending item recorded at interrupt time. argsKey is the
// CANONICAL form of the arguments object: both the re-fire side (raw JSON
// string -> parse -> marshal) and the resume side (the round-tripped
// payload.tool.arguments map -> marshal) produce the same string, since
// encoding/json sorts map keys deterministically. This is what lets the
// resume binding read (name, arguments) straight off payload.tool (§4.4) —
// the domain-neutral envelope always carries name + arguments, so the
// resume binding reads them directly — keeping correlation simple.
// Null byte separator — tool names per the spec cannot contain it
// (lowercase alphanumerics + single hyphens only), so the join is
// unambiguous and collision-free.
func resumeKey(toolName, argsKey string) string {
	return toolName + "\x00" + argsKey
}

// argsKey is the canonical (key-sorted) JSON of a parsed arguments object,
// used as the stable half of resumeKey. nil args canonicalize to "null", an
// empty object to "{}" — consistently on both sides of the resume boundary.
func argsKey(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}

// reuseOrNextItemID returns the original proposal item id + its origin run for
// a re-fired approved tool (so the continuation completes the SAME item), or a
// freshly minted id under the current run otherwise. Primary match is exact
// (name, arguments). When the user EDITED the args at approval the re-fire
// carries different args and misses that key, so it falls back to the unique
// proposal item for this tool name (the runtime parks one awaitable at a time)
// — otherwise the original proposal card would never get its terminal
// item.completed and would hang "in progress" forever.
func (t *translator) reuseOrNextItemID(toolName, argsJSON string) string {
	if t.resume != nil {
		key := resumeKey(toolName, argsKey(protocol.ParseArgs(argsJSON)))
		if orig, ok := t.resume.toolItems[key]; ok {
			delete(t.resume.toolItems, key)
			return orig
		}
		if orig, ok := t.resume.byName[toolName]; ok && orig != "" {
			delete(t.resume.byName, toolName)
			return orig
		}
	}
	return t.nextItemID()
}

// resumeQuestionCompletions terminalizes the question items (ask_user /
// exit_plan_mode) the interrupted run left inProgress. A question is resolved
// by the resume answer (no event re-fires), so the continuation must emit its
// item.completed itself — otherwise the proposal card stays "LIVE" forever
// (API.md §5.2). Emitted once, right after segment.started; the completed item
// keeps the original id + runId and carries the Question payload so items.list
// stays well-formed. No-op for root runs / tool-only resumes.
func (t *translator) resumeQuestionCompletions() []protocol.StreamEvent {
	if t.resume == nil || len(t.resume.questions) == 0 {
		return nil
	}
	out := make([]protocol.StreamEvent, 0, len(t.resume.questions))
	for _, q := range t.resume.questions {
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemCompleted,
			Item: &protocol.Item{
				ID:        q.itemID,
				RunID:     t.runID,
				Status:    protocol.ItemStatusCompleted,
				Type:      protocol.ItemTypeQuestion,
				CreatedAt: time.Now().UTC(),
				Question:  q.question,
			},
		})
	}
	t.resume.questions = nil // emit once
	return out
}
