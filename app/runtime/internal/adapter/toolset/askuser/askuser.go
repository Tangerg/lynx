// Package askuser provides the ask_user tool — the model's channel to ask the
// human a question mid-turn. It uses the same runtime interrupt model as tool
// approval and plan review (one shared flow): the call parks via the runtime
// interrupt abstraction, the question surfaces to the client, and on resume the
// human's answer returns as the tool result so the model continues.
//
// One tool, one package — it depends only on the runtime interrupt abstraction
// and the shared [interrupts.Resolution] vocabulary, so it is a plain
// build-time tool (assembled in toolset.Build, not injected by the engine).
package askuser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/hitl"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
)

const toolName = "ask_user"

// askUserArgs is the model-facing argument shape; it drives the JSON schema
// ([schema]) so the parsed struct and the advertised schema can't drift. It
// mirrors [interrupts.QuestionPrompt] with the LLM-facing copy kept here (out
// of the domain type, which exit_plan_mode reuses with different wording); the
// handler maps it across via [askUserArgs.toPrompt].
type askUserArgs struct {
	Questions []questionArg `json:"questions" jsonschema:"required,minItems=1,maxItems=4" jsonschema_description:"The question(s) to ask the user (1-4)."`
}

type questionArg struct {
	Question    string      `json:"question" jsonschema:"required" jsonschema_description:"The full question text."`
	Header      string      `json:"header,omitempty" jsonschema_description:"Short (<=12 char) label/chip summarizing the question."`
	Options     []optionArg `json:"options,omitempty" jsonschema_description:"2-4 choices for a multiple-choice question. Omit for a free-text answer."`
	MultiSelect bool        `json:"multi_select,omitempty" jsonschema_description:"Allow the user to pick more than one option (only meaningful with options)."`
}

type optionArg struct {
	Label       string `json:"label" jsonschema:"required" jsonschema_description:"The choice shown to the user."`
	Description string `json:"description,omitempty" jsonschema_description:"Optional one-line explanation of the choice."`
}

var schema = pkgjson.MustStringDefSchemaOf(askUserArgs{})

// toPrompt maps the parsed args to the domain prompt type.
func (a askUserArgs) toPrompt() interrupts.QuestionPrompt {
	qs := make([]interrupts.Question, len(a.Questions))
	for i, q := range a.Questions {
		var opts []interrupts.Option
		for _, o := range q.Options {
			opts = append(opts, interrupts.Option{Label: o.Label, Description: o.Description})
		}
		qs[i] = interrupts.Question{Question: q.Question, Header: q.Header, Options: opts, MultiSelect: q.MultiSelect}
	}
	return interrupts.QuestionPrompt{Questions: qs}
}

// New builds the ask_user tool.
func New() chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        toolName,
			Description: "Ask the user a question and wait for their answer. Use when you need a decision, clarification, or information only the user can provide — not for routine progress updates. Give 2-4 `options` for a multiple-choice question (put the recommended one first), or omit `options` for a free-text answer; set `multi_select` when more than one option may apply.",
			InputSchema: schema,
		},
		func(ctx context.Context, arguments string) (string, error) {
			var a askUserArgs
			if err := json.Unmarshal([]byte(arguments), &a); err != nil {
				return "", fmt.Errorf("ask_user: invalid arguments: %w", err)
			}
			if len(a.Questions) == 0 {
				return "", errors.New("ask_user: at least one question is required")
			}
			in := a.toPrompt()
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
	return hitl.Key("ask_user", toolName, arguments)
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
