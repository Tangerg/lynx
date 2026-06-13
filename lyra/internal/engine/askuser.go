package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
)

// QuestionPrompt is the payload surfaced to the client when the model calls
// the ask_user tool — a free-form question awaiting the human's answer. It
// is classified as a "question" interrupt (not "approval") since it is not
// an ApprovalPrompt.
type QuestionPrompt struct {
	Question string `json:"question"`
}

const askUserToolName = "ask_user"

const askUserSchema = `{"type":"object","properties":{"question":{"type":"string","description":"The question to ask the user."}},"required":["question"]}`

// newAskUserTool builds the ask_user tool — the model's channel to ask the
// human a question mid-turn. It rides the SAME HITL interrupt model as tool
// approval and plan review: the call interrupts via [hitl.Interrupt], the
// run parks and surfaces the question, and on resume the tool returns the
// human's answer as its result so the model continues with it. One mental
// model, three flavors.
func newAskUserTool() chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        askUserToolName,
			Description: "Ask the user a question and wait for their answer. Use when you need a decision, clarification, or information only the user can provide.",
			InputSchema: askUserSchema,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in QuestionPrompt
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("ask_user: invalid arguments: %w", err)
			}
			// First pass interrupts (bubbles up, parks); resume returns the
			// human's structured answer at this same call site.
			res, _, err := hitl.Interrupt[interrupts.Resolution](ctx, askUserKey(arguments), in)
			if err != nil {
				return "", err
			}
			return answerText(res.Answer), nil
		},
	)
	return t
}

// askUserKey is the interrupt key for one ask_user call. Keyed by the
// arguments so the recorded answer matches the same call site when the parked
// question is re-presented on resume (mirrors approvalKey).
func askUserKey(arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(askUserToolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return "ask_user." + strconv.FormatUint(h.Sum64(), 16)
}

// answerText renders the structured answer map as the tool's result text.
// Prefers the "text" field (joining multi-value answers a line apiece);
// falls back to a compact JSON rendering.
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
