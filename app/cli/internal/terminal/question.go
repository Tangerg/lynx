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

type questionnaire struct {
	question  agent.Question
	current   int
	responses []questionResponse
}

type questionResponse struct {
	field    agent.QuestionField
	text     string
	single   questionChoice
	multiple []questionChoice
	custom   string
}

type questionChoice struct {
	value  string
	custom bool
}

func offeredQuestionChoice(value string) questionChoice { return questionChoice{value: value} }

func customQuestionChoice() questionChoice { return questionChoice{custom: true} }

func newQuestionnaire(question agent.Question, previous agent.Answer) (*questionnaire, error) {
	if len(question.Fields) == 0 {
		return nil, errors.New("runtime returned a question without fields")
	}
	review := &questionnaire{question: question.Clone(), responses: make([]questionResponse, len(question.Fields))}
	answer, ok := previous.(agent.QuestionAnswer)
	for index, field := range review.question.Fields {
		var values []string
		if ok && index < len(answer.Values) {
			values = answer.Values[index]
		}
		review.responses[index] = newQuestionResponse(field, values)
	}
	return review, nil
}

func newQuestionResponse(field agent.QuestionField, previous []string) questionResponse {
	response := questionResponse{field: field}
	if field.Kind == agent.QuestionSingle && len(field.Options) > 0 {
		response.single = offeredQuestionChoice(field.Options[0].Label)
	}
	response.restore(previous)
	return response
}

func (r *questionResponse) restore(values []string) {
	switch r.field.Kind {
	case agent.QuestionText:
		if len(values) > 0 {
			r.text = values[0]
		}
	case agent.QuestionSingle:
		if len(values) == 0 {
			return
		}
		if fieldOffers(r.field, values[0]) || !r.field.AllowCustom {
			r.single = offeredQuestionChoice(values[0])
			return
		}
		r.single = customQuestionChoice()
		r.custom = values[0]
	case agent.QuestionMulti:
		custom := make([]string, 0, len(values))
		for _, value := range values {
			if fieldOffers(r.field, value) || !r.field.AllowCustom {
				r.multiple = append(r.multiple, offeredQuestionChoice(value))
				continue
			}
			custom = append(custom, value)
		}
		if len(custom) > 0 {
			r.multiple = append(r.multiple, customQuestionChoice())
			r.custom = strings.Join(custom, ", ")
		}
	}
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

func (q *questionnaire) response(index int) *questionResponse {
	if q == nil || index < 0 || index >= len(q.responses) {
		return nil
	}
	return &q.responses[index]
}

func (q *questionnaire) Answer() (agent.QuestionAnswer, error) {
	if q == nil {
		return agent.QuestionAnswer{}, errors.New("questionnaire is not active")
	}
	answer := agent.QuestionAnswer{Values: make([][]string, len(q.question.Fields))}
	for index := range q.responses {
		values, err := q.responses[index].values()
		if err != nil {
			return agent.QuestionAnswer{}, fmt.Errorf("answer question field %d: %w", index+1, err)
		}
		answer.Values[index] = values
	}
	if err := agent.ValidateAnswer(q.question, answer); err != nil {
		return agent.QuestionAnswer{}, fmt.Errorf("answer question: %w", err)
	}
	return answer, nil
}

func (r *questionResponse) values() ([]string, error) {
	switch r.field.Kind {
	case agent.QuestionText:
		value := strings.TrimSpace(r.text)
		if err := requiredText(value); err != nil {
			return nil, err
		}
		return []string{value}, nil
	case agent.QuestionSingle:
		if !r.single.custom {
			if r.single.value == "" {
				return nil, errors.New("choose an option")
			}
			return []string{r.single.value}, nil
		}
		value := strings.TrimSpace(r.custom)
		if err := requiredText(value); err != nil {
			return nil, err
		}
		return []string{value}, nil
	case agent.QuestionMulti:
		values := make([]string, 0, len(r.multiple))
		seen := make(map[string]struct{}, len(r.multiple))
		for _, choice := range r.multiple {
			if choice.custom {
				custom, err := parseCustomChoices(r.custom)
				if err != nil {
					return nil, err
				}
				for _, value := range custom {
					if _, duplicate := seen[value]; duplicate {
						return nil, fmt.Errorf("choice %q is duplicated", value)
					}
					seen[value] = struct{}{}
					values = append(values, value)
				}
				continue
			}
			if _, duplicate := seen[choice.value]; duplicate {
				return nil, fmt.Errorf("choice %q is duplicated", choice.value)
			}
			seen[choice.value] = struct{}{}
			values = append(values, choice.value)
		}
		if len(values) == 0 {
			return nil, errors.New("choose at least one option")
		}
		return values, nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
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
	index, _, ok := review.Current()
	if !ok {
		a.fail(errors.New("questionnaire has no current field"))
		return
	}
	response := review.response(index)
	if response == nil {
		a.fail(errors.New("questionnaire has no response for the current field"))
		return
	}
	fields, err := a.buildQuestionFields(response)
	if err != nil {
		a.fail(err)
		return
	}
	a.showQuestionDialog(review, fields)
}

func (a *app) buildQuestionFields(response *questionResponse) ([]headless.Field, error) {
	specification := response.field
	label := questionFieldLabel(specification)
	switch specification.Kind {
	case agent.QuestionText:
		return []headless.Field{a.buildQuestionText(&response.text, label, "", requiredText)}, nil
	case agent.QuestionSingle:
		fields := []headless.Field{a.buildQuestionSingle(response, specification, label)}
		return a.appendCustomQuestionField(fields, response), nil
	case agent.QuestionMulti:
		fields := []headless.Field{a.buildQuestionMulti(response, specification, label)}
		return a.appendCustomQuestionField(fields, response), nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
}

func (a *app) buildQuestionText(value *string, label, placeholder string, check func(string) error) headless.Field {
	field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check}
	field.Editor().Clipboard = a.loop.Clipboard()
	return field
}

func (a *app) buildQuestionSingle(response *questionResponse, specification agent.QuestionField, label string) headless.Field {
	options := questionOptions(specification)
	field := &headless.Select[questionChoice]{
		Label: label, Value: headless.Bind(&response.single), Rows: min(len(options), 5),
		Same: sameQuestionChoice,
	}
	field.SetOptions(options)
	return field
}

func (a *app) buildQuestionMulti(response *questionResponse, specification agent.QuestionField, label string) headless.Field {
	options := questionOptions(specification)
	field := &headless.MultiSelect[questionChoice]{
		Label: label, Value: headless.Bind(&response.multiple), Rows: min(len(options), 5),
		Same: sameQuestionChoice, Check: requiredQuestionChoices,
	}
	field.SetOptions(options)
	return field
}

func (a *app) appendCustomQuestionField(fields []headless.Field, response *questionResponse) []headless.Field {
	if !response.field.AllowCustom {
		return fields
	}
	placeholder := "Used when “Other” is selected"
	if response.field.Kind == agent.QuestionMulti {
		placeholder += "; separate multiple values with commas"
	}
	check := func(value string) error {
		if !response.choosesCustom() {
			return nil
		}
		_, err := response.values()
		return err
	}
	return append(fields, a.buildQuestionText(&response.custom, "Custom answer", placeholder, check))
}

func questionFieldLabel(specification agent.QuestionField) string {
	if specification.Header == "" {
		return specification.Prompt
	}
	return specification.Header + " — " + specification.Prompt
}

func (r *questionResponse) choosesCustom() bool {
	if r.field.Kind == agent.QuestionSingle {
		return r.single.custom
	}
	for _, choice := range r.multiple {
		if choice.custom {
			return true
		}
	}
	return false
}

func (a *app) showQuestionDialog(review *questionnaire, fields []headless.Field) {
	form := headless.NewForm(fields...)
	var dialog *kit.Dialog
	form.Done = func() {
		if a.questionnaire == review && a.questionDialog == dialog {
			a.advanceQuestionnaire()
		}
	}
	form.GaveUp = func() {
		if a.questionnaire == review && a.questionDialog == dialog {
			a.backOrCancelQuestionnaire()
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Title: strings.Join(nonEmptyStrings([]string{
			a.interactionReview.SubmissionFailure(), review.question.Detail,
		}), "\n"),
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: review.Title(), Body: dressed,
		Where: layout.Placement{Width: 88, Height: formDialogHeight(dressed.Measure(80), len(fields), 18)},
	})
	a.questionDialog = dialog
	dialog.Show()
}

func (a *app) advanceQuestionnaire() {
	review := a.questionnaire
	if review == nil {
		return
	}
	a.questionDialog.Dismiss()
	a.questionDialog = nil
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
	a.questionDialog = nil
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
		a.questionDialog = nil
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

func questionOptions(field agent.QuestionField) []headless.Option[questionChoice] {
	out := make([]headless.Option[questionChoice], 0, len(field.Options)+1)
	for _, option := range field.Options {
		label := option.Label
		if option.Description != "" {
			label += " — " + option.Description
		}
		if option.Preview != "" {
			label += " · " + option.Preview
		}
		out = append(out, headless.Option[questionChoice]{Label: label, Value: offeredQuestionChoice(option.Label)})
	}
	if field.AllowCustom {
		out = append(out, headless.Option[questionChoice]{Label: "Other — provide a custom answer", Value: customQuestionChoice()})
	}
	return out
}

func fieldOffers(field agent.QuestionField, value string) bool {
	for _, option := range field.Options {
		if option.Label == value {
			return true
		}
	}
	return false
}

func sameQuestionChoice(left, right questionChoice) bool {
	return left == right
}

func requiredQuestionChoices(values []questionChoice) error {
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

func parseCustomChoices(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	choices := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		choice := strings.TrimSpace(part)
		if choice == "" {
			continue
		}
		if _, duplicate := seen[choice]; duplicate {
			return nil, fmt.Errorf("choice %q is duplicated", choice)
		}
		seen[choice] = struct{}{}
		choices = append(choices, choice)
	}
	if len(choices) == 0 {
		return nil, errors.New("choose at least one option")
	}
	return choices, nil
}
