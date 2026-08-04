// Package askuser exposes ask_user over the runtime's resumable question flow.
package askuser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// askUserArgs is the model-facing argument shape; [toolcontract.NewFunc] derives
// the JSON schema from it and decodes calls back into it, so the advertised
// schema and parsed value cannot drift. The handler maps its LLM-oriented names
// into the application-owned [runs.QuestionPrompt] contract.
type askUserArgs struct {
	Questions []questionArg `json:"questions" jsonschema:"required,minItems=1,maxItems=4" jsonschema_description:"The question(s) to ask the user (1-4)."`
}

type questionArg struct {
	Question    string      `json:"question" jsonschema:"minLength=1" jsonschema_description:"The full question text."`
	Header      string      `json:"header,omitempty" jsonschema:"maxLength=12" jsonschema_description:"Short label of at most 12 characters that identifies the question."`
	Options     []optionArg `json:"options,omitempty" jsonschema:"minItems=2,maxItems=4" jsonschema_description:"Two to four choices for a multiple-choice question. Omit for a free-text answer."`
	MultiSelect bool        `json:"multi_select,omitempty" jsonschema_description:"Allow the user to pick more than one option (only meaningful with options)."`
}

type optionArg struct {
	Label       string `json:"label" jsonschema:"minLength=1" jsonschema_description:"The choice shown to the user."`
	Description string `json:"description,omitempty" jsonschema_description:"Optional one-line explanation of the choice."`
}

func (a askUserArgs) validate() error {
	if len(a.Questions) == 0 {
		return errors.New("at least one question is required")
	}
	return nil
}

func (a askUserArgs) toFields() []runs.QuestionFieldSpec {
	fields := make([]runs.QuestionFieldSpec, len(a.Questions))
	for i, q := range a.Questions {
		var opts []runs.QuestionOptionSpec
		for _, o := range q.Options {
			opts = append(opts, runs.QuestionOptionSpec{Label: o.Label, Description: o.Description})
		}
		fields[i] = runs.QuestionFieldSpec{
			Prompt: q.Question, Header: q.Header, Options: opts,
			Multiple: q.MultiSelect, AllowCustom: len(opts) > 0,
		}
	}
	return fields
}

func (a askUserArgs) arguments() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("ask_user: encode arguments: %w", err)
	}
	return string(b), nil
}

type asker struct {
	interrupt runs.InterruptFunc
}

// New builds the ask_user tool.
func New(interrupt runs.InterruptFunc) (toolcontract.Tool, error) {
	if interrupt == nil {
		interrupt = runs.InterruptUnavailable
	}
	t := &asker{interrupt: interrupt}
	return toolcontract.NewFunc[askUserArgs, string](
		toolcontract.FuncConfig{
			Name:        catalog.AskUser,
			Description: "Ask the user one to four questions and wait for their answers. Use this only when progress requires a decision, clarification, or information only the user can provide, not for routine progress updates. Give two to four `options` for a multiple-choice question and put the recommended option first; the user may still provide a custom answer. Omit `options` for free text, and set `multi_select` only when options are present and more than one may apply.",
		},
		t.ask,
	)
}

func (t *asker) ask(ctx context.Context, a askUserArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", fmt.Errorf("ask_user: %w", err)
	}
	arguments, err := a.arguments()
	if err != nil {
		return "", err
	}
	in := runs.QuestionPrompt{
		ToolName:  catalog.AskUser,
		Arguments: arguments,
		Fields:    a.toFields(),
	}
	pending := runs.Interrupt{Kind: execution.QuestionInterrupt, Question: &in}
	if err := pending.Validate(); err != nil {
		return "", fmt.Errorf("ask_user: %w", err)
	}
	// First pass interrupts (bubbles up, parks); resume returns the human's
	// structured answers at this same call site.
	res, err := t.interrupt(ctx,
		interrupts.InterruptKey(execution.QuestionInterrupt.String(), catalog.AskUser, arguments),
		pending,
	)
	if err != nil {
		return "", err
	}
	return answerText(in, res.Answers), nil
}

// answerText renders the human's answers as the tool's result text, pairing
// each question with its answer by the same stable order. A
// single question returns just its answer (no label noise); multiple questions
// return "header: answer" lines so the model can tell them apart. Multi-select
// answers are comma-joined.
func answerText(in runs.QuestionPrompt, answers [][]string) string {
	if len(in.Fields) == 1 {
		return strings.Join(answers[0], "\n")
	}
	var b strings.Builder
	for i, q := range in.Fields {
		label := q.Header
		if label == "" {
			label = q.Prompt
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(answers[i], ", "))
	}
	return strings.TrimSpace(b.String())
}
