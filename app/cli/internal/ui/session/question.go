package session

import (
	"errors"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func (a *app) openQuestion(question client.Question) {
	cloned := question
	a.question = &cloned
	a.questionText = make(map[string]*string)
	a.questionMulti = make(map[string]*[]string)
	a.questionBool = make(map[string]*bool)

	fields := make([]headless.Field, 0, len(question.Fields))
	for _, specification := range question.Fields {
		label := specification.Label
		if specification.Description != "" {
			label += " — " + specification.Description
		}
		switch specification.Kind {
		case client.QuestionText:
			value := new(string)
			a.questionText[specification.ID] = value
			field := &headless.Text{Label: label, Placeholder: specification.Placeholder, Value: headless.Bind(value)}
			if specification.Required {
				field.Check = requiredText
			}
			field.Editor().Clipboard = a.loop.Clipboard()
			fields = append(fields, field)
		case client.QuestionSingle:
			value := new(string)
			options := questionOptions(specification.Options)
			for _, option := range specification.Options {
				if option.Recommended {
					*value = option.Value
					break
				}
			}
			if *value == "" && len(options) > 0 {
				*value = options[0].Value
			}
			a.questionText[specification.ID] = value
			field := &headless.Select[string]{Label: label, Value: headless.Bind(value), Rows: min(len(options), 5)}
			field.SetOptions(options)
			if specification.Required {
				field.Check = requiredText
			}
			fields = append(fields, field)
		case client.QuestionMulti:
			value := new([]string)
			a.questionMulti[specification.ID] = value
			field := &headless.MultiSelect[string]{Label: label, Value: headless.Bind(value), Rows: min(len(specification.Options), 5)}
			field.SetOptions(questionOptions(specification.Options))
			if specification.Required {
				field.Check = func(values []string) error {
					if len(values) == 0 {
						return errors.New("choose at least one option")
					}
					return nil
				}
			}
			fields = append(fields, field)
		case client.QuestionBool:
			value := new(bool)
			a.questionBool[specification.ID] = value
			fields = append(fields, &headless.Confirm{Label: label, Value: headless.Bind(value), Yes: "yes", No: "no"})
		default:
			a.fail(errors.New("runtime returned an unsupported question field kind"))
			return
		}
	}
	if len(fields) == 0 {
		a.fail(errors.New("runtime returned a question without fields"))
		return
	}
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
	answer := client.QuestionAnswer{Canceled: canceled}
	if !canceled {
		answer.Values = make(map[string][]string, len(question.Fields))
		for _, field := range question.Fields {
			switch field.Kind {
			case client.QuestionText, client.QuestionSingle:
				if value := a.questionText[field.ID]; value != nil {
					answer.Values[field.ID] = []string{*value}
				}
			case client.QuestionMulti:
				if value := a.questionMulti[field.ID]; value != nil {
					answer.Values[field.ID] = append([]string(nil), (*value)...)
				}
			case client.QuestionBool:
				if value := a.questionBool[field.ID]; value != nil && *value {
					answer.Values[field.ID] = []string{"true"}
				} else {
					answer.Values[field.ID] = []string{"false"}
				}
			}
		}
	}
	a.question = nil
	a.questionDialog.Dismiss()
	a.status.active("resuming")
	a.syncAnimation()
	a.resumeInteraction(question.InterruptID, answer)
}

func questionOptions(options []client.QuestionOption) []headless.Option[string] {
	out := make([]headless.Option[string], 0, len(options))
	for _, option := range options {
		label := option.Label
		if label == "" {
			label = option.Value
		}
		if option.Recommended {
			label += " (recommended)"
		}
		out = append(out, headless.Option[string]{Label: label, Value: option.Value})
	}
	return out
}

func requiredText(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("an answer is required")
	}
	return nil
}
