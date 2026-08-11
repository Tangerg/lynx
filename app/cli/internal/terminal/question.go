package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (a *app) openQuestion(question agent.Question) {
	a.prepareQuestion(question)
	fields, err := a.buildQuestionFields(question.Fields)
	if err != nil {
		a.fail(err)
		return
	}
	a.showQuestionDialog(question, fields)
}

func (a *app) prepareQuestion(question agent.Question) {
	cloned := agent.CloneInteraction(question).(agent.Question)
	a.question = &cloned
	a.questionText = make(map[int]*string)
	a.questionMulti = make(map[int]*[]string)
}

func (a *app) buildQuestionFields(specifications []agent.QuestionField) ([]headless.Field, error) {
	fields := make([]headless.Field, 0, len(specifications))
	for i, specification := range specifications {
		field, err := a.buildQuestionField(i, specification)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	if len(fields) == 0 {
		return nil, errors.New("runtime returned a question without fields")
	}
	return fields, nil
}

func (a *app) buildQuestionField(index int, specification agent.QuestionField) (headless.Field, error) {
	label := questionFieldLabel(specification)
	switch specification.Kind {
	case agent.QuestionText:
		return a.buildQuestionText(index, label, ""), nil
	case agent.QuestionSingle:
		if specification.AllowCustom {
			return a.buildQuestionText(index, label, choicePlaceholder(specification.Options)), nil
		}
		return a.buildQuestionSingle(index, specification, label), nil
	case agent.QuestionMulti:
		if specification.AllowCustom {
			return a.buildQuestionText(index, label+" (comma-separated)", choicePlaceholder(specification.Options)), nil
		}
		return a.buildQuestionMulti(index, specification, label), nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
}

func (a *app) buildQuestionText(index int, label, placeholder string) headless.Field {
	value := new(string)
	a.questionText[index] = value
	field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: requiredText}
	field.Editor().Clipboard = a.loop.Clipboard()
	return field
}

func (a *app) buildQuestionSingle(index int, specification agent.QuestionField, label string) headless.Field {
	value := new(string)
	options := questionOptions(specification.Options)
	if len(options) > 0 {
		*value = options[0].Value
	}
	a.questionText[index] = value
	field := &headless.Select[string]{Label: label, Value: headless.Bind(value), Rows: min(len(options), 5), Check: requiredText}
	field.SetOptions(options)
	return field
}

func (a *app) buildQuestionMulti(index int, specification agent.QuestionField, label string) headless.Field {
	value := new([]string)
	a.questionMulti[index] = value
	field := &headless.MultiSelect[string]{Label: label, Value: headless.Bind(value), Rows: min(len(specification.Options), 5), Check: requiredChoices}
	field.SetOptions(questionOptions(specification.Options))
	return field
}

func questionFieldLabel(specification agent.QuestionField) string {
	if specification.Header == "" {
		return specification.Prompt
	}
	return specification.Header + " — " + specification.Prompt
}

func choicePlaceholder(options []agent.QuestionOption) string {
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	return strings.Join(labels, " / ")
}

func (a *app) showQuestionDialog(question agent.Question, fields []headless.Field) {
	keys := headless.DefaultFormKeys()
	form := headless.NewForm(fields...)
	form.Keys = keys
	form.Gap = 1
	form.Done = func() { a.answerQuestion(false) }
	form.GaveUp = func() { a.answerQuestion(true) }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Title: question.Detail, Hints: []keymap.Action{headless.FocusNext, headless.Submit, headless.Cancel},
	})
	a.questionDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: question.Title, Body: dressed,
		Where: layout.Placement{Width: 88, Height: min(26, max(10, dressed.Measure(80)+2)), Margin: 1},
	})
	a.questionDialog.Show()
}

func (a *app) answerQuestion(canceled bool) {
	question := a.question
	if question == nil {
		return
	}
	a.question = nil
	a.questionDialog.Dismiss()
	if canceled {
		a.abortInteractions("question canceled by the terminal user")
		return
	}
	answer, err := a.questionAnswer(*question)
	if err != nil {
		a.fail(err)
		return
	}
	a.acceptInteractionAnswer(question.ItemID, answer)
}

func (a *app) questionAnswer(question agent.Question) (agent.QuestionAnswer, error) {
	answer := agent.QuestionAnswer{Values: make([][]string, len(question.Fields))}
	for i, field := range question.Fields {
		answer.Values[i] = a.questionValues(i, field)
	}
	if err := agent.ValidateAnswer(question, answer); err != nil {
		return agent.QuestionAnswer{}, fmt.Errorf("answer question: %w", err)
	}
	return answer, nil
}

func (a *app) questionValues(index int, field agent.QuestionField) []string {
	if value := a.questionMulti[index]; value != nil {
		return append([]string(nil), (*value)...)
	}
	value := a.questionText[index]
	if value == nil {
		return nil
	}
	if field.Kind == agent.QuestionMulti && field.AllowCustom {
		parts := strings.Split(*value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	}
	return []string{strings.TrimSpace(*value)}
}

func questionOptions(options []agent.QuestionOption) []headless.Option[string] {
	out := make([]headless.Option[string], 0, len(options))
	for _, option := range options {
		label := option.Label
		if option.Description != "" {
			label += " — " + option.Description
		}
		if option.Preview != "" {
			label += " · " + option.Preview
		}
		out = append(out, headless.Option[string]{Label: label, Value: option.Label})
	}
	return out
}

func requiredChoices(values []string) error {
	if len(values) == 0 {
		return errors.New("choose at least one option")
	}
	return nil
}

func requiredText(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("an answer is required")
	}
	return nil
}
