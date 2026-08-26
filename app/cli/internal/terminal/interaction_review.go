package terminal

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// interactionReview owns the terminal-side draft of a runtime interaction
// batch. Runtime interactions remain immutable; only the answers and cursor
// change until the user commits the complete batch once.
type interactionReview struct {
	items             []agent.Interaction
	answers           []agent.Answer
	current           int
	submissionFailure string
}

func newInteractionReview(items []agent.Interaction) (*interactionReview, error) {
	if err := agent.ValidateInteractions(items); err != nil {
		return nil, err
	}
	cloned := agent.CloneInteractions(items)
	return &interactionReview{items: cloned, answers: make([]agent.Answer, len(cloned))}, nil
}

func restoreInteractionReview(items []agent.Interaction, responses []agent.InterruptAnswer) (*interactionReview, error) {
	review, err := newInteractionReview(items)
	if err != nil {
		return nil, err
	}
	if len(responses) != len(items) {
		return nil, errors.New("interaction response count does not match review")
	}
	for index, response := range responses {
		if response.ItemID != agent.InteractionItemID(items[index]) {
			return nil, fmt.Errorf("interaction response %d targets another item", index+1)
		}
		if err := review.Record(response.Answer); err != nil {
			return nil, fmt.Errorf("restore interaction response %d: %w", index+1, err)
		}
		if !review.Advance() && index+1 < len(responses) {
			return nil, fmt.Errorf("restore interaction response %d did not advance", index+1)
		}
	}
	return review, nil
}

func (i *interactionReview) Current() (agent.Interaction, bool) {
	if i == nil || i.current < 0 || i.current >= len(i.items) {
		return nil, false
	}
	return agent.CloneInteraction(i.items[i.current]), true
}

func (i *interactionReview) CurrentAnswer() agent.Answer {
	if i == nil || i.current < 0 || i.current >= len(i.answers) {
		return nil
	}
	return agent.CloneAnswer(i.answers[i.current])
}

func (i *interactionReview) Record(answer agent.Answer) error {
	item, ok := i.Current()
	if !ok {
		return errors.New("interaction review has no current item")
	}
	if err := agent.ValidateAnswer(item, answer); err != nil {
		return err
	}
	i.answers[i.current] = agent.CloneAnswer(answer)
	return nil
}

func (i *interactionReview) Advance() bool {
	if i == nil || i.current >= len(i.items) || i.answers[i.current] == nil {
		return false
	}
	i.current++
	return i.current < len(i.items)
}

func (i *interactionReview) Back() bool {
	if i == nil || i.current <= 0 {
		return false
	}
	if i.current >= len(i.items) {
		i.current = len(i.items) - 1
	} else {
		i.current--
	}
	return true
}

func (i *interactionReview) completed() bool {
	return i != nil && len(i.items) > 0 && i.current == len(i.items)
}

func (i *interactionReview) ReportSubmissionFailure(err error) {
	if i == nil || err == nil {
		return
	}
	i.submissionFailure = err.Error()
}

func (i *interactionReview) SubmissionFailure() string {
	if i == nil {
		return ""
	}
	return i.submissionFailure
}

func (i *interactionReview) Reviewing() bool {
	return i != nil && len(i.items) > 1 && i.completed()
}

func (i *interactionReview) Position() (current, total int) {
	if i == nil {
		return 0, 0
	}
	return min(i.current+1, len(i.items)), len(i.items)
}

func (i *interactionReview) Responses() ([]agent.InterruptAnswer, error) {
	if i == nil || len(i.items) == 0 {
		return nil, errors.New("interaction review is empty")
	}
	responses := make([]agent.InterruptAnswer, len(i.items))
	for index, item := range i.items {
		if i.answers[index] == nil {
			return nil, fmt.Errorf("interaction %d has no answer", index+1)
		}
		responses[index] = agent.InterruptAnswer{
			ItemID: agent.InteractionItemID(item),
			Answer: agent.CloneAnswer(i.answers[index]),
		}
	}
	return responses, nil
}

func (i *interactionReview) Items() []agent.Interaction {
	if i == nil {
		return nil
	}
	return agent.CloneInteractions(i.items)
}

func (i *interactionReview) Answers() []agent.Answer {
	if i == nil {
		return nil
	}
	answers := slices.Clone(i.answers)
	for index := range answers {
		answers[index] = agent.CloneAnswer(answers[index])
	}
	return answers
}
