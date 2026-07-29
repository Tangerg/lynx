package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// resolveResumeResponses validates exact item coverage and the kind-specific
// answer schema, then binds every decision to its exact executor suspension.
// Output follows the pending barrier's canonical order, independent of request
// ordering, so every downstream layer observes one representation of the set.
func resolveResumeResponses(pending interrupts.Pending, responses []ResumeResponse) ([]interrupts.SuspensionAnswer, error) {
	open := make(map[string]transcript.Interrupt, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		if interrupt.ItemID == "" {
			return nil, fmt.Errorf("%w: open interrupt has no item id", ErrInvalidInterruptResponse)
		}
		if _, exists := open[interrupt.ItemID]; exists {
			return nil, fmt.Errorf("%w: duplicate open item %q", ErrInvalidInterruptResponse, interrupt.ItemID)
		}
		open[interrupt.ItemID] = interrupt
	}
	if len(open) == 0 {
		return nil, ErrInterruptNotOpen
	}

	seen := make(map[string]struct{}, len(responses))
	resolutions := make(map[string]interrupts.Resolution, len(responses))
	for _, response := range responses {
		interrupt, exists := open[response.ItemID]
		if !exists {
			return nil, fmt.Errorf("%w: item %q", ErrInterruptNotOpen, response.ItemID)
		}
		if _, duplicate := seen[response.ItemID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate response for item %q", ErrInvalidInterruptResponse, response.ItemID)
		}
		seen[response.ItemID] = struct{}{}

		var (
			itemResolution interrupts.Resolution
			err            error
		)
		switch interrupt.Kind {
		case execution.ApprovalInterrupt:
			itemResolution, err = resolveApprovalResponse(interrupt, response)
		case execution.QuestionInterrupt:
			itemResolution, err = resolveQuestionResponse(interrupt, response)
		default:
			err = fmt.Errorf("unknown open interrupt kind %d", interrupt.Kind)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: item %q: %w", ErrInvalidInterruptResponse, response.ItemID, err)
		}
		resolutions[response.ItemID] = itemResolution
	}
	if len(seen) != len(open) {
		return nil, fmt.Errorf(
			"%w: responses cover %d of %d open items",
			ErrInvalidInterruptResponse, len(seen), len(open),
		)
	}
	if len(pending.Suspensions) != len(pending.Interrupts) {
		return nil, fmt.Errorf(
			"%w: pending barrier has %d suspension bindings for %d items",
			ErrInvalidInterruptResponse,
			len(pending.Suspensions),
			len(pending.Interrupts),
		)
	}
	answers := make([]interrupts.SuspensionAnswer, len(pending.Suspensions))
	for index, binding := range pending.Suspensions {
		resolution, ok := resolutions[binding.InterruptItemID]
		if !ok {
			return nil, fmt.Errorf(
				"%w: suspension binding names unanswered item %q",
				ErrInvalidInterruptResponse,
				binding.InterruptItemID,
			)
		}
		answers[index] = interrupts.SuspensionAnswer{
			InterruptItemID: binding.InterruptItemID,
			ProcessID:       binding.ProcessID,
			SuspensionID:    binding.SuspensionID,
			Resolution:      resolution,
		}
	}
	return answers, nil
}

func resolveApprovalResponse(interrupt transcript.Interrupt, response ResumeResponse) (interrupts.Resolution, error) {
	if response.Kind != ApprovalResponseKind || response.Approval == nil || response.Question != nil {
		return interrupts.Resolution{}, errors.New("approval response is required")
	}
	approval := response.Approval
	if approval.RememberScope != "" && !approval.RememberScope.Valid() {
		return interrupts.Resolution{}, fmt.Errorf("unknown remember scope %q", approval.RememberScope)
	}
	if approval.RememberScope != "" && (interrupt.Approval == nil || !interrupt.Approval.Rememberable) {
		return interrupts.Resolution{}, errors.New("approval cannot be remembered")
	}
	if approval.Arguments != "" {
		if !approval.Approved {
			return interrupts.Resolution{}, errors.New("denial cannot edit arguments")
		}
		if err := validateArguments(approval.Arguments); err != nil {
			return interrupts.Resolution{}, fmt.Errorf("edited arguments: %w", err)
		}
	}
	if approval.Approved && strings.TrimSpace(approval.Reason) != "" {
		return interrupts.Resolution{}, errors.New("approval cannot carry a denial reason")
	}
	return interrupts.Resolution{
		Approved:      approval.Approved,
		Arguments:     approval.Arguments,
		Reason:        strings.TrimSpace(approval.Reason),
		RememberScope: approval.RememberScope,
	}, nil
}

func resolveQuestionResponse(interrupt transcript.Interrupt, response ResumeResponse) (interrupts.Resolution, error) {
	if response.Kind != QuestionResponseKind || response.Question == nil || response.Approval != nil {
		return interrupts.Resolution{}, errors.New("question response is required")
	}
	if interrupt.Question == nil || len(interrupt.Question.Fields) == 0 {
		return interrupts.Resolution{}, errors.New("open question has no fields")
	}
	answers := response.Question.Answers
	if len(answers) != len(interrupt.Question.Fields) {
		return interrupts.Resolution{}, fmt.Errorf(
			"answers cover %d of %d fields", len(answers), len(interrupt.Question.Fields),
		)
	}
	for _, field := range interrupt.Question.Fields {
		values, ok := answers[field.Name]
		if !ok {
			return interrupts.Resolution{}, fmt.Errorf("missing answer for field %q", field.Name)
		}
		if err := validateQuestionAnswer(field, values); err != nil {
			return interrupts.Resolution{}, fmt.Errorf("field %q: %w", field.Name, err)
		}
	}
	for name := range answers {
		found := false
		for _, field := range interrupt.Question.Fields {
			if field.Name == name {
				found = true
				break
			}
		}
		if !found {
			return interrupts.Resolution{}, fmt.Errorf("unknown answer field %q", name)
		}
	}
	return interrupts.Resolution{Approved: true, Answer: cloneAnswers(answers)}, nil
}

func validateQuestionAnswer(field transcript.QuestionField, values []string) error {
	switch field.Kind {
	case transcript.QuestionText:
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return errors.New("one non-empty text value is required")
		}
	case transcript.QuestionChoice:
		if len(values) == 0 {
			return errors.New("at least one choice is required")
		}
		if !field.Multiple && len(values) != 1 {
			return errors.New("exactly one choice is required")
		}
		allowed := make(map[string]struct{}, len(field.Options))
		for _, option := range field.Options {
			allowed[option.Label] = struct{}{}
		}
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if _, ok := allowed[value]; !ok {
				return fmt.Errorf("unknown choice %q", value)
			}
			if _, duplicate := seen[value]; duplicate {
				return errors.New("duplicate choices are not allowed")
			}
			seen[value] = struct{}{}
		}
	default:
		return fmt.Errorf("unknown question field kind %d", field.Kind)
	}
	return nil
}

func cloneAnswers(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for name, values := range in {
		out[name] = slices.Clone(values)
	}
	return out
}
