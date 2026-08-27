package terminal

import (
	"fmt"

	"github.com/Tangerg/oolong/components/headless"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

type trackedQuestion struct {
	runID string
	id    headless.BlockID
	block *questionBlock
}

// acceptQuestions reveals the durable Question replacements acknowledged by a
// successful resume command. Validation is all-or-nothing so a malformed local
// projection cannot leave only part of a multi-question set visible.
func (t *transcriptView) acceptQuestions(blocks []agent.Block) error {
	type acceptance struct {
		key      string
		tracked  trackedQuestion
		question agent.Question
	}
	accepted := make([]acceptance, 0, len(blocks))
	for _, block := range blocks {
		if block.Kind != agent.BlockQuestion || block.Question == nil {
			return fmt.Errorf("terminal transcript: accepted interaction block %s is not a question", block.ID)
		}
		key := transcriptBlockKey(block.RunID, block.ID)
		tracked, exists := t.pendingQuestions[key]
		if !exists {
			return fmt.Errorf("terminal transcript: accepted question block %s is not pending", block.ID)
		}
		if err := tracked.block.validateAccepted(*block.Question); err != nil {
			return fmt.Errorf("terminal transcript: %w", err)
		}
		accepted = append(accepted, acceptance{key: key, tracked: tracked, question: block.Question.Clone()})
	}
	for _, item := range accepted {
		item.tracked.block.accept(item.question)
		t.content.Changed(item.tracked.id)
		t.content.Finish(item.tracked.id)
		delete(t.pendingQuestions, item.key)
	}
	if len(accepted) > 0 {
		t.refreshSearch()
		t.announceSelection()
	}
	return nil
}

func (t *transcriptView) finishPendingQuestions(runID string) {
	for key, question := range t.pendingQuestions {
		if runID != "" && question.runID != runID {
			continue
		}
		t.content.Finish(question.id)
		delete(t.pendingQuestions, key)
	}
}

// reconcilePendingQuestions closes presentation lifetimes after a cold read.
// An unanswered durable Question remains hidden and retained only when the same
// snapshot exposes it as an open interaction. Canceled historical questions are
// still intentionally invisible, but must not pin the transcript forever.
func (t *transcriptView) reconcilePendingQuestions(interactions []agent.Interaction) error {
	open := make(map[string]struct{}, len(interactions))
	for _, interaction := range interactions {
		question, ok := interaction.(agent.Question)
		if !ok {
			continue
		}
		key := transcriptBlockKey(question.RunID, question.ItemID)
		if _, pending := t.pendingQuestions[key]; !pending {
			return fmt.Errorf("terminal transcript: open question block %s has no pending presentation", question.ItemID)
		}
		open[key] = struct{}{}
	}
	for key, question := range t.pendingQuestions {
		if _, remainsOpen := open[key]; remainsOpen {
			continue
		}
		t.content.Finish(question.id)
		delete(t.pendingQuestions, key)
	}
	return nil
}
