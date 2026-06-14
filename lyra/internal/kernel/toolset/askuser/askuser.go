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
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
)

const toolName = "ask_user"

const schema = `{"type":"object","properties":{` +
	`"questions":{"type":"array","minItems":1,"maxItems":4,"description":"The question(s) to ask the user (1-4).","items":{"type":"object","properties":{` +
	`"question":{"type":"string","description":"The full question text."},` +
	`"header":{"type":"string","description":"Short (≤12 char) label/chip summarizing the question."},` +
	`"options":{"type":"array","description":"2-4 choices for a multiple-choice question. Omit for a free-text answer.","items":{"type":"object","properties":{` +
	`"label":{"type":"string","description":"The choice shown to the user."},` +
	`"description":{"type":"string","description":"Optional one-line explanation of the choice."}` +
	`},"required":["label"]}},` +
	`"multi_select":{"type":"boolean","description":"Allow the user to pick more than one option (only meaningful with options)."}` +
	`},"required":["question"]}}` +
	`},"required":["questions"]}`

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
			if len(in.Questions) == 0 {
				return "", errors.New("ask_user: at least one question is required")
			}
			// First pass interrupts (bubbles up, parks); resume returns the
			// human's structured answers at this same call site.
			res, _, err := hitl.Interrupt[interrupts.Resolution](ctx, key(arguments), in)
			if err != nil {
				return "", err
			}
			return answerText(in, res.Answer), nil
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

// answerText renders the human's answers as the tool's result text, pairing
// each question with its answer (keyed by [interrupts.QuestionFieldName]). A
// single question returns just its answer (no label noise); multiple questions
// return "header: answer" lines so the model can tell them apart. Multi-select
// answers are comma-joined.
func answerText(in interrupts.QuestionPrompt, answer map[string][]string) string {
	if len(in.Questions) == 1 {
		return strings.Join(answer[interrupts.QuestionFieldName(0)], "\n")
	}
	var b strings.Builder
	for i, q := range in.Questions {
		label := q.Header
		if label == "" {
			label = q.Question
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(answer[interrupts.QuestionFieldName(i)], ", "))
	}
	return strings.TrimSpace(b.String())
}
