// Package askuser provides the ask_user tool — the model's channel to ask the
// human a question mid-turn. It rides the SAME HITL interrupt model as tool
// approval and plan review (one mental model, three flavors): the call parks
// the run via [hitl.Interrupt], the question surfaces to the client, and on
// resume the human's answer returns as the tool result so the model continues.
//
// One tool, one package — it depends only on the SDK HITL mechanism and the
// shared [interrupts.Resolution] vocabulary, so it is a plain build-time tool
// (assembled in toolset.Build, not injected by the engine).
package askuser

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
)

const toolName = "ask_user"

const schema = `{"type":"object","properties":{"question":{"type":"string","description":"The question to ask the user."}},"required":["question"]}`

// New builds the ask_user tool.
func New() chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        toolName,
			Description: "Ask the user a question and wait for their answer. Use when you need a decision, clarification, or information only the user can provide.",
			InputSchema: schema,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in interrupts.QuestionPrompt
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("ask_user: invalid arguments: %w", err)
			}
			// First pass interrupts (bubbles up, parks); resume returns the
			// human's structured answer at this same call site.
			res, _, err := hitl.Interrupt[interrupts.Resolution](ctx, key(arguments), in)
			if err != nil {
				return "", err
			}
			return answerText(res.Answer), nil
		},
	)
	return t
}

// key is the interrupt key for one ask_user call. Keyed by the arguments so the
// recorded answer matches the same call site when the parked question is
// re-presented on resume (mirrors the approval gate's key).
func key(arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(toolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return toolName + "." + strconv.FormatUint(h.Sum64(), 16)
}

// answerText renders the structured answer map as the tool's result text.
// Prefers the "text" field (joining multi-value answers a line apiece); falls
// back to a compact JSON rendering.
func answerText(answer map[string][]string) string {
	if answer == nil {
		return ""
	}
	if v, ok := answer["text"]; ok && len(v) > 0 {
		return strings.Join(v, "\n")
	}
	b, _ := json.Marshal(answer)
	return string(b)
}
