package agent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

func InteractionItemID(interaction Interaction) string {
	switch item := interaction.(type) {
	case Approval:
		return item.ItemID
	case Question:
		return item.ItemID
	default:
		return ""
	}
}

func InteractionRunID(interaction Interaction) string {
	switch item := interaction.(type) {
	case Approval:
		return item.RunID
	case Question:
		return item.RunID
	default:
		return ""
	}
}

func ValidateInteraction(interaction Interaction) error {
	switch item := interaction.(type) {
	case Approval:
		return item.Validate()
	case Question:
		if item.Answered() {
			return errors.New("interaction question already has accepted answers")
		}
		return item.Validate()
	case nil:
		return errors.New("interaction is nil")
	default:
		return fmt.Errorf("interaction %T is unsupported", interaction)
	}
}

func ValidateInteractions(interactions []Interaction) error {
	if len(interactions) == 0 {
		return errors.New("interactions are empty")
	}
	seen := make(map[string]struct{}, len(interactions))
	for i, interaction := range interactions {
		if err := ValidateInteraction(interaction); err != nil {
			return fmt.Errorf("interaction %d: %w", i+1, err)
		}
		id := InteractionItemID(interaction)
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("interaction item id %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (a Approval) Validate() error {
	var problems []error
	if strings.TrimSpace(a.RunID) == "" {
		problems = append(problems, errors.New("run id is empty"))
	}
	if strings.TrimSpace(a.ItemID) == "" {
		problems = append(problems, errors.New("item id is empty"))
	}
	if strings.TrimSpace(a.Title) == "" {
		problems = append(problems, errors.New("title is empty"))
	}
	if a.Tool == nil {
		problems = append(problems, errors.New("tool is absent"))
	} else {
		if err := a.Tool.Validate(); err != nil {
			problems = append(problems, err)
		}
		if a.Tool.Status != ToolRunning {
			problems = append(problems, errors.New("tool is not running"))
		}
	}
	if a.Risk != "" && !slices.Contains([]ApprovalRisk{ApprovalRiskLow, ApprovalRiskMedium, ApprovalRiskHigh}, a.Risk) {
		problems = append(problems, fmt.Errorf("risk %q is invalid", a.Risk))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	return nil
}

// Clone returns an approval with no mutable tool projection shared with the caller.
func (a Approval) Clone() Approval {
	if a.Tool != nil {
		tool := a.Tool.Clone()
		a.Tool = &tool
	}
	return a
}

// Equal reports whether two approvals ask for the same decision about the same
// projected tool invocation.
func (a Approval) Equal(other Approval) bool {
	if a.RunID != other.RunID || a.ItemID != other.ItemID || a.Title != other.Title || a.Detail != other.Detail ||
		a.Diff != other.Diff || a.Risk != other.Risk || a.RuleHint != other.RuleHint ||
		a.Rememberable != other.Rememberable || (a.Tool == nil) != (other.Tool == nil) {
		return false
	}
	return a.Tool == nil || a.Tool.Equal(*other.Tool)
}

func (q Question) Validate() error {
	var problems []error
	if strings.TrimSpace(q.RunID) == "" {
		problems = append(problems, errors.New("run id is empty"))
	}
	if strings.TrimSpace(q.ItemID) == "" {
		problems = append(problems, errors.New("item id is empty"))
	}
	if strings.TrimSpace(q.Title) == "" {
		problems = append(problems, errors.New("title is empty"))
	}
	if len(q.Fields) == 0 {
		problems = append(problems, errors.New("fields are empty"))
	}
	for i, field := range q.Fields {
		if err := field.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("field %d: %w", i+1, err))
		}
	}
	if q.Answered() {
		if err := validateQuestionAnswer(q, QuestionAnswer{Values: q.Answers}); err != nil {
			problems = append(problems, fmt.Errorf("accepted answer: %w", err))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("question: %w", err)
	}
	return nil
}

// Answered reports whether this question is an authoritative completed
// transcript fact rather than a pending interaction.
func (q Question) Answered() bool { return q.Answers != nil }

// Accept returns the durable transcript form of a pending question after the
// runtime has accepted its complete ordered response. The pending value is not
// mutated, so an editor can retain its draft independently from the accepted
// conversation fact.
func (q Question) Accept(answer QuestionAnswer) (Question, error) {
	if q.Answered() {
		return Question{}, errors.New("question already has accepted answers")
	}
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	if err := validateQuestionAnswer(q, answer); err != nil {
		return Question{}, err
	}
	accepted := q.Clone()
	accepted.Answers = cloneAnswerValues(answer.Values)
	return accepted, nil
}

// Clone returns a question with no mutable field or option storage shared with the caller.
func (q Question) Clone() Question { return cloneQuestion(q) }

// Equal reports whether two questions present the same ordered fields. Field
// order is semantic because QuestionAnswer uses the same order on the wire.
func (q Question) Equal(other Question) bool {
	return q.RunID == other.RunID && q.ItemID == other.ItemID && q.Title == other.Title && q.Detail == other.Detail &&
		q.Answered() == other.Answered() &&
		slices.EqualFunc(q.Fields, other.Fields, func(left, right QuestionField) bool {
			return left.Equal(right)
		}) && equalAnswerValues(q.Answers, other.Answers)
}

func (q QuestionField) Validate() error {
	if strings.TrimSpace(q.Prompt) == "" {
		return errors.New("prompt is empty")
	}
	if utf8.RuneCountInString(q.Header) > 12 {
		return errors.New("header is longer than 12 characters")
	}
	switch q.Kind {
	case QuestionText:
		if len(q.Options) != 0 || q.AllowCustom {
			return errors.New("text field carries choice options")
		}
	case QuestionSingle, QuestionMulti:
		if len(q.Options) < 2 {
			return errors.New("choice field has fewer than two options")
		}
		seen := make(map[string]struct{}, len(q.Options))
		for i, option := range q.Options {
			if strings.TrimSpace(option.Label) == "" {
				return fmt.Errorf("option %d has no label", i+1)
			}
			if _, duplicate := seen[option.Label]; duplicate {
				return fmt.Errorf("option label %q is duplicated", option.Label)
			}
			seen[option.Label] = struct{}{}
		}
	default:
		return fmt.Errorf("kind %q is invalid", q.Kind)
	}
	return nil
}

// Equal reports whether two fields accept and present the same answer space.
func (q QuestionField) Equal(other QuestionField) bool {
	return q.Prompt == other.Prompt && q.Header == other.Header && q.Kind == other.Kind &&
		q.AllowCustom == other.AllowCustom && slices.Equal(q.Options, other.Options)
}

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
		if !item.Rememberable && provided.Remember != RememberNone {
			return errors.New("approval cannot be remembered")
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

func (a ApprovalAnswer) Validate() error {
	if !slices.Contains([]ApprovalDecision{ApprovalApprove, ApprovalDeny}, a.Decision) {
		return fmt.Errorf("approval answer: decision %q is invalid", a.Decision)
	}
	if !slices.Contains([]RememberScope{RememberNone, RememberSession, RememberProject, RememberGlobal}, a.Remember) {
		return fmt.Errorf("approval answer: remember scope %q is invalid", a.Remember)
	}
	if a.ArgumentOverride != nil {
		if a.Decision != ApprovalApprove {
			return errors.New("approval answer: denied tool carries an argument override")
		}
		if err := a.ArgumentOverride.Validate(); err != nil {
			return fmt.Errorf("approval answer: %w", err)
		}
	}
	return nil
}

func validateQuestionAnswer(question Question, answer QuestionAnswer) error {
	if len(answer.Values) != len(question.Fields) {
		return fmt.Errorf("question answer: got %d fields, want %d", len(answer.Values), len(question.Fields))
	}
	for i, field := range question.Fields {
		if err := validateQuestionValues(field, answer.Values[i]); err != nil {
			return fmt.Errorf("question answer: field %d: %w", i+1, err)
		}
	}
	return nil
}

func validateQuestionValues(field QuestionField, values []string) error {
	if len(values) == 0 {
		return errors.New("an answer is required")
	}
	switch field.Kind {
	case QuestionText:
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return errors.New("text requires one non-empty value")
		}
	case QuestionSingle:
		if len(values) != 1 {
			return errors.New("single choice accepts one value")
		}
		if !field.AllowCustom && !questionOffers(field, values[0]) {
			return fmt.Errorf("option %q is not offered", values[0])
		}
	case QuestionMulti:
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if !field.AllowCustom && !questionOffers(field, value) {
				return fmt.Errorf("option %q is not offered", value)
			}
			if _, duplicate := seen[value]; duplicate {
				return fmt.Errorf("option %q is duplicated", value)
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

func questionOffers(field QuestionField, value string) bool {
	return slices.ContainsFunc(field.Options, func(option QuestionOption) bool { return option.Label == value })
}

func CloneInteraction(interaction Interaction) Interaction {
	switch item := interaction.(type) {
	case Approval:
		return item.Clone()
	case Question:
		return item.Clone()
	default:
		return nil
	}
}

func cloneQuestion(question Question) Question {
	question.Fields = slices.Clone(question.Fields)
	for i := range question.Fields {
		question.Fields[i].Options = slices.Clone(question.Fields[i].Options)
	}
	question.Answers = cloneAnswerValues(question.Answers)
	return question
}

func cloneAnswerValues(values [][]string) [][]string {
	if values == nil {
		return nil
	}
	cloned := make([][]string, len(values))
	for i, answers := range values {
		cloned[i] = slices.Clone(answers)
	}
	return cloned
}

func equalAnswerValues(left, right [][]string) bool {
	return slices.EqualFunc(left, right, func(left, right []string) bool {
		return slices.Equal(left, right)
	})
}

func CloneInteractions(interactions []Interaction) []Interaction {
	out := make([]Interaction, len(interactions))
	for i, interaction := range interactions {
		out[i] = CloneInteraction(interaction)
	}
	return out
}

func CloneAnswer(answer Answer) Answer {
	switch item := answer.(type) {
	case ApprovalAnswer:
		item.ArgumentOverride = item.ArgumentOverride.Clone()
		return item
	case QuestionAnswer:
		cloned := item
		cloned.Values = cloneAnswerValues(item.Values)
		return cloned
	default:
		return nil
	}
}

// AnswerEqual reports whether two interaction answers carry the same complete
// decision value, including edited tool arguments and ordered question values.
func AnswerEqual(left, right Answer) bool {
	switch typed := left.(type) {
	case ApprovalAnswer:
		other, ok := right.(ApprovalAnswer)
		return ok && typed.Decision == other.Decision && typed.Remember == other.Remember &&
			typed.Reason == other.Reason && typed.ArgumentOverride.Equal(other.ArgumentOverride)
	case QuestionAnswer:
		other, ok := right.(QuestionAnswer)
		return ok && equalAnswerValues(typed.Values, other.Values)
	default:
		return left == nil && right == nil
	}
}

// InteractionsEqual reports whether two ordered pending interaction sets are
// the same runtime decision surface.
func InteractionsEqual(left, right []Interaction) bool {
	return slices.EqualFunc(left, right, func(left, right Interaction) bool {
		switch typed := left.(type) {
		case Approval:
			other, ok := right.(Approval)
			return ok && typed.Equal(other)
		case Question:
			other, ok := right.(Question)
			return ok && typed.Equal(other)
		default:
			return left == nil && right == nil
		}
	})
}
