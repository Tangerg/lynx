package terminal

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type questionnaire struct {
	question agent.Question
	current  int
	text     []*string
	multiple []*[]string
}

func newQuestionnaire(question agent.Question, previous agent.Answer) (*questionnaire, error) {
	if len(question.Fields) == 0 {
		return nil, errors.New("runtime returned a question without fields")
	}
	review := &questionnaire{
		question: question.Clone(),
		text:     make([]*string, len(question.Fields)),
		multiple: make([]*[]string, len(question.Fields)),
	}
	for index, field := range review.question.Fields {
		if field.Kind == agent.QuestionMulti && !field.AllowCustom {
			review.multiple[index] = new([]string)
			continue
		}
		review.text[index] = new(string)
	}
	answer, ok := previous.(agent.QuestionAnswer)
	if !ok {
		return review, nil
	}
	for index, values := range answer.Values {
		if index >= len(review.question.Fields) {
			break
		}
		if target := review.multiple[index]; target != nil {
			*target = slices.Clone(values)
			continue
		}
		*review.text[index] = strings.Join(values, ", ")
	}
	return review, nil
}

func (q *questionnaire) Current() (int, agent.QuestionField, bool) {
	if q == nil || q.current < 0 || q.current >= len(q.question.Fields) {
		return 0, agent.QuestionField{}, false
	}
	return q.current, q.question.Fields[q.current], true
}

func (q *questionnaire) Advance() bool {
	if q == nil || q.current+1 >= len(q.question.Fields) {
		return false
	}
	q.current++
	return true
}

func (q *questionnaire) Back() bool {
	if q == nil || q.current == 0 {
		return false
	}
	q.current--
	return true
}

func (q *questionnaire) Title() string {
	if q == nil || len(q.question.Fields) <= 1 {
		if q == nil {
			return ""
		}
		return q.question.Title
	}
	return fmt.Sprintf("%s · %d/%d", q.question.Title, q.current+1, len(q.question.Fields))
}

func (q *questionnaire) Text(index int) *string {
	if q == nil || index < 0 || index >= len(q.text) {
		return nil
	}
	return q.text[index]
}

func (q *questionnaire) Multiple(index int) *[]string {
	if q == nil || index < 0 || index >= len(q.multiple) {
		return nil
	}
	return q.multiple[index]
}

func (q *questionnaire) Answer() (agent.QuestionAnswer, error) {
	if q == nil {
		return agent.QuestionAnswer{}, errors.New("questionnaire is not active")
	}
	answer := agent.QuestionAnswer{Values: make([][]string, len(q.question.Fields))}
	for index, field := range q.question.Fields {
		answer.Values[index] = q.values(index, field)
	}
	if err := agent.ValidateAnswer(q.question, answer); err != nil {
		return agent.QuestionAnswer{}, fmt.Errorf("answer question: %w", err)
	}
	return answer, nil
}

func (q *questionnaire) values(index int, field agent.QuestionField) []string {
	if value := q.multiple[index]; value != nil {
		return slices.Clone(*value)
	}
	value := q.text[index]
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

func (a *app) openQuestion(question agent.Question) {
	review, err := newQuestionnaire(question, a.interactionReview.CurrentAnswer())
	if err != nil {
		a.fail(err)
		return
	}
	a.questionnaire = review
	a.openQuestionField()
}

func (a *app) openQuestionField() {
	review := a.questionnaire
	index, specification, ok := review.Current()
	if !ok {
		a.fail(errors.New("questionnaire has no current field"))
		return
	}
	field, err := a.buildQuestionField(review, index, specification)
	if err != nil {
		a.fail(err)
		return
	}
	a.showQuestionDialog(review, field)
}

func (a *app) buildQuestionField(review *questionnaire, index int, specification agent.QuestionField) (headless.Field, error) {
	label := questionFieldLabel(specification)
	switch specification.Kind {
	case agent.QuestionText:
		return a.buildQuestionText(review, index, label, ""), nil
	case agent.QuestionSingle:
		if specification.AllowCustom {
			return a.buildQuestionText(review, index, label, choicePlaceholder(specification.Options)), nil
		}
		return a.buildQuestionSingle(review, index, specification, label), nil
	case agent.QuestionMulti:
		if specification.AllowCustom {
			return a.buildQuestionText(review, index, label+" (comma-separated)", choicePlaceholder(specification.Options)), nil
		}
		return a.buildQuestionMulti(review, index, specification, label), nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
}

func (a *app) buildQuestionText(review *questionnaire, index int, label, placeholder string) headless.Field {
	field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(review.Text(index)), Check: requiredText}
	field.Editor().Clipboard = a.loop.Clipboard()
	return field
}

func (a *app) buildQuestionSingle(review *questionnaire, index int, specification agent.QuestionField, label string) headless.Field {
	value := review.Text(index)
	options := questionOptions(specification.Options)
	if *value == "" && len(options) > 0 {
		*value = options[0].Value
	}
	field := &headless.Select[string]{Label: label, Value: headless.Bind(value), Rows: min(len(options), 5), Check: requiredText}
	field.SetOptions(options)
	return field
}

func (a *app) buildQuestionMulti(review *questionnaire, index int, specification agent.QuestionField, label string) headless.Field {
	field := &headless.MultiSelect[string]{Label: label, Value: headless.Bind(review.Multiple(index)), Rows: min(len(specification.Options), 5), Check: requiredChoices}
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

func (a *app) showQuestionDialog(review *questionnaire, field headless.Field) {
	form := headless.NewForm(field)
	form.Done = a.advanceQuestionnaire
	form.GaveUp = a.backOrCancelQuestionnaire
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Title: review.question.Detail, Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.questionDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: review.Title(), Body: dressed,
		Where: layout.Placement{Width: 88, Height: min(18, max(8, dressed.Measure(80)+2))},
	})
	a.questionDialog.Show()
}

func (a *app) advanceQuestionnaire() {
	review := a.questionnaire
	if review == nil {
		return
	}
	a.questionDialog.Dismiss()
	if review.Advance() {
		a.openQuestionField()
		return
	}
	a.finishQuestionnaire(false)
}

func (a *app) backOrCancelQuestionnaire() {
	review := a.questionnaire
	if review == nil {
		return
	}
	a.questionDialog.Dismiss()
	if review.Back() {
		a.openQuestionField()
		return
	}
	a.finishQuestionnaire(true)
}

func (a *app) finishQuestionnaire(canceled bool) {
	review := a.questionnaire
	if review == nil {
		return
	}
	if a.questionDialog != nil {
		a.questionDialog.Dismiss()
	}
	if canceled {
		a.questionnaire = nil
		if a.backInteraction() {
			return
		}
		a.abortInteractions("question canceled by the terminal user")
		return
	}
	answer, err := review.Answer()
	if err != nil {
		a.fail(err)
		return
	}
	if err := a.interactionReview.Record(answer); err != nil {
		a.fail(fmt.Errorf("record question: %w", err))
		return
	}
	a.questionnaire = nil
	a.advanceInteractionReview()
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
