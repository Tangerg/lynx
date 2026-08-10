package terminal

import (
	"errors"
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
	a.questionText = make(map[string]*string)
	a.questionMulti = make(map[string]*[]string)
	a.questionBool = make(map[string]*bool)
}

func (a *app) buildQuestionFields(specifications []agent.QuestionField) ([]headless.Field, error) {
	fields := make([]headless.Field, 0, len(specifications))
	for _, specification := range specifications {
		field, err := a.buildQuestionField(specification)
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

func (a *app) buildQuestionField(specification agent.QuestionField) (headless.Field, error) {
	label := questionFieldLabel(specification)
	switch specification.Kind {
	case agent.QuestionText:
		return a.buildQuestionText(specification, label), nil
	case agent.QuestionSingle:
		return a.buildQuestionSingle(specification, label), nil
	case agent.QuestionMulti:
		return a.buildQuestionMulti(specification, label), nil
	case agent.QuestionBool:
		value := new(bool)
		a.questionBool[specification.ID] = value
		return &headless.Confirm{Label: label, Value: headless.Bind(value), Yes: "yes", No: "no"}, nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
}

func (a *app) buildQuestionText(specification agent.QuestionField, label string) headless.Field {
	value := new(string)
	a.questionText[specification.ID] = value
	field := &headless.Text{Label: label, Placeholder: specification.Placeholder, Value: headless.Bind(value)}
	if specification.Required {
		field.Check = requiredText
	}
	field.Editor().Clipboard = a.loop.Clipboard()
	return field
}

func (a *app) buildQuestionSingle(specification agent.QuestionField, label string) headless.Field {
	value := new(string)
	options := questionOptions(specification.Options)
	*value = defaultQuestionOption(specification.Options)
	a.questionText[specification.ID] = value
	field := &headless.Select[string]{Label: label, Value: headless.Bind(value), Rows: min(len(options), 5)}
	field.SetOptions(options)
	if specification.Required {
		field.Check = requiredText
	}
	return field
}

func (a *app) buildQuestionMulti(specification agent.QuestionField, label string) headless.Field {
	value := new([]string)
	a.questionMulti[specification.ID] = value
	field := &headless.MultiSelect[string]{Label: label, Value: headless.Bind(value), Rows: min(len(specification.Options), 5)}
	field.SetOptions(questionOptions(specification.Options))
	if specification.Required {
		field.Check = requiredChoices
	}
	return field
}

func questionFieldLabel(specification agent.QuestionField) string {
	if specification.Description == "" {
		return specification.Label
	}
	return specification.Label + " — " + specification.Description
}

func defaultQuestionOption(options []agent.QuestionOption) string {
	for _, option := range options {
		if option.Recommended {
			return option.Value
		}
	}
	if len(options) > 0 {
		return options[0].Value
	}
	return ""
}

func (a *app) showQuestionDialog(question agent.Question, fields []headless.Field) {
	keys := headless.DefaultFormKeys()
	form := headless.NewForm(fields...)
	form.Keys = keys
	form.Gap = 1
	form.Done = func() { a.answerQuestion(false) }
	form.GaveUp = func() { a.answerQuestion(true) }
	dressed := kit.NewForm(a.transcript.theme, a.transcript.glyphs, form)
	dressed.Title = question.Detail
	dressed.Keys = keys
	dressed.Hints = []keymap.Action{headless.FocusNext, headless.Submit, headless.Cancel}
	a.questionDialog = kit.NewDialog(&a.stack, a.transcript.theme, a.transcript.glyphs, question.Title, dressed)
	a.questionDialog.Panel().Where = layout.Placement{Width: 88, Height: min(26, max(10, dressed.Measure(80)+2)), Margin: 1}
	a.questionDialog.Show()
}

func (a *app) answerQuestion(canceled bool) {
	question := a.question
	if question == nil {
		return
	}
	answer := a.questionAnswer(*question, canceled)
	a.question = nil
	a.questionDialog.Dismiss()
	a.status.active("resuming")
	a.syncAnimation()
	a.resumeInteraction(question.InterruptID, answer)
}

func (a *app) questionAnswer(question agent.Question, canceled bool) agent.QuestionAnswer {
	answer := agent.QuestionAnswer{Canceled: canceled}
	if canceled {
		return answer
	}
	answer.Values = make(map[string][]string, len(question.Fields))
	for _, field := range question.Fields {
		answer.Values[field.ID] = a.questionValues(field)
	}
	return answer
}

func (a *app) questionValues(field agent.QuestionField) []string {
	switch field.Kind {
	case agent.QuestionText, agent.QuestionSingle:
		if value := a.questionText[field.ID]; value != nil {
			return []string{*value}
		}
	case agent.QuestionMulti:
		if value := a.questionMulti[field.ID]; value != nil {
			return append([]string(nil), (*value)...)
		}
	case agent.QuestionBool:
		if value := a.questionBool[field.ID]; value != nil && *value {
			return []string{"true"}
		}
		return []string{"false"}
	default:
	}
	return nil
}

func questionOptions(options []agent.QuestionOption) []headless.Option[string] {
	out := make([]headless.Option[string], 0, len(options))
	for _, option := range options {
		label := option.Label
		if label == "" {
			label = option.Value
		}
		if option.Recommended {
			label += " (recommended)"
		}
		if option.Description != "" {
			label += " — " + option.Description
		}
		out = append(out, headless.Option[string]{Label: label, Value: option.Value})
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
