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

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/tools"
)

const toolName = "ask_user"

// askUserArgs is the model-facing argument shape; [tools.New] derives
// the JSON schema from it and decodes calls back into it, so the advertised
// schema and parsed value cannot drift. It mirrors [interrupts.QuestionPrompt]
// with the LLM-facing copy kept here (out of the domain type, which
// exit_plan_mode reuses with different wording); the handler maps it across via
// [askUserArgs.toPrompt].
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

func (a askUserArgs) validate() error {
	if len(a.Questions) == 0 {
		return errors.New("at least one question is required")
	}
	return nil
}

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

func (a askUserArgs) key() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("ask_user: encode interrupt key: %w", err)
	}
	return interrupts.InterruptKey("ask_user", toolName, string(b)), nil
}

type tool struct {
	interrupt interrupts.Interruption
}

// New builds the ask_user tool.
func New(interrupt interrupts.Interruption) (tools.Tool, error) {
	if interrupt == nil {
		interrupt = interrupts.NoInterruption
	}
	t := &tool{interrupt: interrupt}
	return tools.New[askUserArgs, string](
		tools.Config{
			Name:        toolName,
			Description: "Ask the user a question and wait for their answer. Use when you need a decision, clarification, or information only the user can provide - not for routine progress updates. Give 2-4 `options` for a multiple-choice question (put the recommended one first), or omit `options` for a free-text answer; set `multi_select` when more than one option may apply.",
		},
		t.ask,
	)
}

func (t *tool) ask(ctx context.Context, a askUserArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", fmt.Errorf("ask_user: %w", err)
	}
	key, err := a.key()
	if err != nil {
		return "", err
	}
	in := a.toPrompt()
	// First pass interrupts (bubbles up, parks); resume returns the human's
	// structured answers at this same call site.
	res, _, err := t.interrupt(ctx, key, in)
	if err != nil {
		return "", err
	}
	return answerText(in, res.Answer), nil
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
