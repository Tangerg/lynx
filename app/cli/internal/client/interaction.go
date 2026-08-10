package client

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// InteractionID returns the durable identity shared by every interaction kind.
func InteractionID(interaction Interaction) string {
	switch item := interaction.(type) {
	case Approval:
		return item.InterruptID
	case Question:
		return item.InterruptID
	default:
		return ""
	}
}

// ValidateInteraction rejects an incomplete or unknown runtime interaction.
func ValidateInteraction(interaction Interaction) error {
	switch item := interaction.(type) {
	case Approval:
		return item.Validate()
	case Question:
		return item.Validate()
	case nil:
		return errors.New("interaction is nil")
	default:
		return fmt.Errorf("interaction %T is unsupported", interaction)
	}
}

// Validate checks the fields required to present and answer an approval.
func (a Approval) Validate() error {
	var problems []error
	if strings.TrimSpace(a.InterruptID) == "" {
		problems = append(problems, errors.New("interrupt id is empty"))
	}
	if strings.TrimSpace(a.Title) == "" {
		problems = append(problems, errors.New("title is empty"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	return nil
}

// Validate checks field identities and the closed question vocabulary before a
// terminal adapter builds widgets from it.
func (q Question) Validate() error {
	var problems []error
	if strings.TrimSpace(q.InterruptID) == "" {
		problems = append(problems, errors.New("interrupt id is empty"))
	}
	if strings.TrimSpace(q.Title) == "" {
		problems = append(problems, errors.New("title is empty"))
	}
	if len(q.Fields) == 0 {
		problems = append(problems, errors.New("fields are empty"))
	}
	seen := make(map[string]struct{}, len(q.Fields))
	for i, field := range q.Fields {
		if err := validateQuestionField(field, seen); err != nil {
			problems = append(problems, fmt.Errorf("field %d: %w", i+1, err))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("question: %w", err)
	}
	return nil
}

func validateQuestionField(field QuestionField, seen map[string]struct{}) error {
	id := strings.TrimSpace(field.ID)
	if id == "" {
		return errors.New("id is empty")
	}
	if _, duplicate := seen[id]; duplicate {
		return fmt.Errorf("id %q is duplicated", id)
	}
	seen[id] = struct{}{}
	if strings.TrimSpace(field.Label) == "" {
		return fmt.Errorf("%q has no label", id)
	}
	return validateQuestionFieldShape(field, id)
}

func validateQuestionFieldShape(field QuestionField, id string) error {
	switch field.Kind {
	case QuestionText, QuestionBool:
		if len(field.Options) != 0 {
			return fmt.Errorf("%q of kind %q cannot carry options", id, field.Kind)
		}
	case QuestionSingle, QuestionMulti:
		if len(field.Options) == 0 {
			return fmt.Errorf("%q of kind %q has no options", id, field.Kind)
		}
		return validateQuestionOptions(field, id)
	default:
		return fmt.Errorf("%q has invalid kind %q", id, field.Kind)
	}
	return nil
}

func validateQuestionOptions(field QuestionField, id string) error {
	values := make(map[string]struct{}, len(field.Options))
	recommended := 0
	for i, option := range field.Options {
		value := strings.TrimSpace(option.Value)
		if value == "" {
			return fmt.Errorf("%q option %d has no value", id, i+1)
		}
		if _, duplicate := values[value]; duplicate {
			return fmt.Errorf("%q repeats option %q", id, value)
		}
		values[value] = struct{}{}
		if option.Recommended {
			recommended++
		}
	}
	if field.Kind == QuestionSingle && recommended > 1 {
		return fmt.Errorf("%q has more than one recommended option", id)
	}
	return nil
}

// ValidateAnswer checks that answer matches the active interaction exactly.
func ValidateAnswer(interaction Interaction, answer Answer) error {
	if err := ValidateInteraction(interaction); err != nil {
		return err
	}
	switch item := interaction.(type) {
	case Approval:
		provided, ok := answer.(ApprovalAnswer)
		if !ok {
			return errors.New("approval requires an approval answer")
		}
		return provided.Validate()
	case Question:
		provided, ok := answer.(QuestionAnswer)
		if !ok {
			return errors.New("question requires a question answer")
		}
		return validateQuestionAnswer(item, provided)
	default:
		return fmt.Errorf("interaction %T is unsupported", interaction)
	}
}

// Validate checks the closed approval decision and remembered-rule scope.
func (a ApprovalAnswer) Validate() error {
	if !slices.Contains([]ApprovalDecision{ApprovalAllow, ApprovalDeny}, a.Decision) {
		return fmt.Errorf("approval answer: decision %q is invalid", a.Decision)
	}
	if a.Remember != "" && !slices.Contains([]RememberScope{RememberNone, RememberSession, RememberProject, RememberGlobal}, a.Remember) {
		return fmt.Errorf("approval answer: remember scope %q is invalid", a.Remember)
	}
	if a.Decision == ApprovalDeny && a.Remember != "" && a.Remember != RememberNone {
		return errors.New("approval answer: a denial cannot create an allow rule")
	}
	return nil
}

func validateQuestionAnswer(question Question, answer QuestionAnswer) error {
	if answer.Canceled {
		if len(answer.Values) != 0 {
			return errors.New("question answer: canceled answer carries values")
		}
		return nil
	}
	fields := make(map[string]QuestionField, len(question.Fields))
	for _, field := range question.Fields {
		fields[field.ID] = field
	}
	for id := range answer.Values {
		if _, known := fields[id]; !known {
			return fmt.Errorf("question answer: unknown field %q", id)
		}
	}
	for _, field := range question.Fields {
		values := answer.Values[field.ID]
		if err := validateQuestionValues(field, values); err != nil {
			return fmt.Errorf("question answer: field %q: %w", field.ID, err)
		}
	}
	return nil
}

func validateQuestionValues(field QuestionField, values []string) error {
	if len(values) == 0 {
		if field.Required {
			return errors.New("an answer is required")
		}
		return nil
	}
	switch field.Kind {
	case QuestionText:
		return validateTextValue(field.Required, values)
	case QuestionBool:
		return validateBooleanValue(values)
	case QuestionSingle:
		return validateSingleValue(field, values)
	case QuestionMulti:
		return validateMultipleValues(field, values)
	default:
		return fmt.Errorf("kind %q is invalid", field.Kind)
	}
}

func validateTextValue(required bool, values []string) error {
	if len(values) != 1 {
		return errors.New("text accepts one value")
	}
	if required && strings.TrimSpace(values[0]) == "" {
		return errors.New("an answer is required")
	}
	return nil
}

func validateBooleanValue(values []string) error {
	if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
		return errors.New("boolean requires exactly true or false")
	}
	return nil
}

func validateSingleValue(field QuestionField, values []string) error {
	if len(values) != 1 {
		return errors.New("single choice accepts one value")
	}
	if !questionOffers(field, values[0]) {
		return fmt.Errorf("option %q is not offered", values[0])
	}
	return nil
}

func validateMultipleValues(field QuestionField, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !questionOffers(field, value) {
			return fmt.Errorf("option %q is not offered", value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("option %q is duplicated", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func questionOffers(field QuestionField, value string) bool {
	return slices.ContainsFunc(field.Options, func(option QuestionOption) bool { return option.Value == value })
}

// CloneInteraction returns an interaction with no mutable slices shared with
// the caller.
func CloneInteraction(interaction Interaction) Interaction {
	switch item := interaction.(type) {
	case Approval:
		return item
	case Question:
		cloned := item
		cloned.Fields = make([]QuestionField, len(item.Fields))
		for i, field := range item.Fields {
			cloned.Fields[i] = field
			cloned.Fields[i].Options = slices.Clone(field.Options)
		}
		return cloned
	default:
		return nil
	}
}

// CloneAnswer returns an answer with no mutable maps or slices shared with the
// caller.
func CloneAnswer(answer Answer) Answer {
	switch item := answer.(type) {
	case ApprovalAnswer:
		return item
	case QuestionAnswer:
		cloned := QuestionAnswer{Canceled: item.Canceled}
		if item.Values != nil {
			cloned.Values = make(map[string][]string, len(item.Values))
			for id, values := range item.Values {
				cloned.Values[id] = slices.Clone(values)
			}
		}
		return cloned
	default:
		return nil
	}
}

// EqualAnswers reports semantic equality for the closed answer vocabulary.
func EqualAnswers(a, b Answer) bool {
	switch first := a.(type) {
	case ApprovalAnswer:
		second, ok := b.(ApprovalAnswer)
		return ok && first == second
	case QuestionAnswer:
		second, ok := b.(QuestionAnswer)
		return ok && equalQuestionAnswers(first, second)
	default:
		return false
	}
}

func equalQuestionAnswers(first, second QuestionAnswer) bool {
	if first.Canceled != second.Canceled || len(first.Values) != len(second.Values) {
		return false
	}
	for id, values := range first.Values {
		other, exists := second.Values[id]
		if !exists || !slices.Equal(values, other) {
			return false
		}
	}
	return true
}
