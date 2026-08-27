package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/feedback"
)

func (a *app) RecordFeedback(argument string) error {
	if a.feedback == nil {
		return errors.New("this runtime composition has no feedback service")
	}
	ratingText, note, _ := strings.Cut(strings.TrimSpace(argument), " ")
	rating, err := feedback.ParseRating(ratingText)
	if err != nil {
		return errors.New("usage: /feedback <positive|negative> [note]")
	}
	runID, itemID := latestAssistantTarget(a.conversation.Blocks())
	signal := feedback.Signal{
		SessionID: a.session.ID, RunID: runID, ItemID: itemID,
		Rating: rating, Text: strings.TrimSpace(note),
	}
	if err := signal.Validate(); err != nil {
		return err
	}
	a.status.note("recording feedback")
	if !a.runApplicationOperation(feedbackOperation, false,
		func(ctx context.Context) (feedback.Signal, error) { return signal, a.feedback.Record(ctx, signal) },
		func(recorded feedback.Signal, err error) {
			if err != nil {
				a.message("record feedback failed: " + err.Error())
				return
			}
			target := "session"
			if recorded.ItemID != "" {
				target = "assistant item " + shortIdentity(recorded.ItemID)
			} else if recorded.RunID != "" {
				target = "run " + shortIdentity(recorded.RunID)
			}
			a.message("feedback recorded · " + string(recorded.Rating) + " · " + target)
		},
	) {
		return errors.New("another feedback operation is running")
	}
	return nil
}

func latestAssistantTarget(blocks []agent.Block) (string, string) {
	for index := len(blocks) - 1; index >= 0; index-- {
		block := blocks[index]
		if block.Kind == agent.BlockAssistant && block.Status != agent.BlockStatusRunning {
			return block.RunID, block.ID
		}
	}
	return "", ""
}
