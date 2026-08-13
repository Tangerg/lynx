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

func (r *interactionReview) Current() (agent.Interaction, bool) {
	if r == nil || r.current < 0 || r.current >= len(r.items) {
		return nil, false
	}
	return agent.CloneInteraction(r.items[r.current]), true
}

func (r *interactionReview) CurrentAnswer() agent.Answer {
	if r == nil || r.current < 0 || r.current >= len(r.answers) {
		return nil
	}
	return agent.CloneAnswer(r.answers[r.current])
}

func (r *interactionReview) Record(answer agent.Answer) error {
	item, ok := r.Current()
	if !ok {
		return errors.New("interaction review has no current item")
	}
	if err := agent.ValidateAnswer(item, answer); err != nil {
		return err
	}
	r.answers[r.current] = agent.CloneAnswer(answer)
	return nil
}

func (r *interactionReview) Advance() bool {
	if r == nil || r.current >= len(r.items) || r.answers[r.current] == nil {
		return false
	}
	r.current++
	return r.current < len(r.items)
}

func (r *interactionReview) Back() bool {
	if r == nil || r.current <= 0 {
		return false
	}
	if r.current >= len(r.items) {
		r.current = len(r.items) - 1
	} else {
		r.current--
	}
	return true
}

func (r *interactionReview) completed() bool {
	return r != nil && len(r.items) > 0 && r.current == len(r.items)
}

func (r *interactionReview) ReportSubmissionFailure(err error) {
	if r == nil || err == nil {
		return
	}
	r.submissionFailure = err.Error()
}

func (r *interactionReview) SubmissionFailure() string {
	if r == nil {
		return ""
	}
	return r.submissionFailure
}

func (r *interactionReview) Reviewing() bool {
	return r != nil && len(r.items) > 1 && r.completed()
}

func (r *interactionReview) Position() (current, total int) {
	if r == nil {
		return 0, 0
	}
	return min(r.current+1, len(r.items)), len(r.items)
}

func (r *interactionReview) Responses() ([]agent.InterruptAnswer, error) {
	if r == nil || len(r.items) == 0 {
		return nil, errors.New("interaction review is empty")
	}
	responses := make([]agent.InterruptAnswer, len(r.items))
	for i, item := range r.items {
		if r.answers[i] == nil {
			return nil, fmt.Errorf("interaction %d has no answer", i+1)
		}
		responses[i] = agent.InterruptAnswer{
			ItemID: agent.InteractionItemID(item),
			Answer: agent.CloneAnswer(r.answers[i]),
		}
	}
	return responses, nil
}

func (r *interactionReview) Items() []agent.Interaction {
	if r == nil {
		return nil
	}
	return agent.CloneInteractions(r.items)
}

func (r *interactionReview) Answers() []agent.Answer {
	if r == nil {
		return nil
	}
	answers := slices.Clone(r.answers)
	for i := range answers {
		answers[i] = agent.CloneAnswer(answers[i])
	}
	return answers
}
