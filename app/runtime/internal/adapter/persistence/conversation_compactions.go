package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/internal/application/conversations"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

type conversationHistory interface {
	Count(ctx context.Context, sessionID string) (int, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
}

type conversationRuns interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
	RebaseMessageMark(ctx context.Context, expected, replacement run.Run) error
}

// ConversationCompactions applies an Application-decided conversation rewrite
// and all of its Run-watermark replacements in one storage transaction.
type ConversationCompactions struct {
	history conversationHistory
	runs    conversationRuns
	tx      Transactor
}

func NewConversationCompactions(history conversationHistory, runs conversationRuns, tx Transactor) *ConversationCompactions {
	return &ConversationCompactions{history: history, runs: runs, tx: tx}
}

var _ conversations.CompactionStore = (*ConversationCompactions)(nil)

func (c *ConversationCompactions) ListRuns(ctx context.Context, sessionID string) ([]run.Run, error) {
	if c == nil || c.runs == nil {
		return nil, errors.New("persistence: conversation compaction Run store is unavailable")
	}
	return c.runs.ListRuns(ctx, sessionID)
}

func (c *ConversationCompactions) ApplyCompaction(ctx context.Context, plan conversations.CompactionPlan) error {
	if c == nil || c.history == nil || c.runs == nil || c.tx == nil {
		return errors.New("persistence: conversation compaction dependencies are unavailable")
	}
	if plan.SessionID == "" {
		return errors.New("persistence: conversation compaction session ID is required")
	}
	return c.tx(ctx, func(ctx context.Context) error {
		count, err := c.history.Count(ctx, plan.SessionID)
		if err != nil {
			return err
		}
		if count != plan.Compaction.ExpectedCount() {
			return fmt.Errorf(
				"persistence: conversation compaction message count changed from %d to %d",
				plan.Compaction.ExpectedCount(), count,
			)
		}
		current, err := c.runs.ListRuns(ctx, plan.SessionID)
		if err != nil {
			return err
		}
		if len(current) != len(plan.Runs) {
			return fmt.Errorf(
				"persistence: conversation compaction Run set changed from %d to %d records",
				len(plan.Runs), len(current),
			)
		}
		for index, planned := range plan.Runs {
			if !current[index].Equal(planned.Expected) {
				return fmt.Errorf("persistence: conversation compaction Run %q changed", planned.Expected.ID())
			}
			if planned.Expected.SessionID() != plan.SessionID || planned.Replacement.SessionID() != plan.SessionID {
				return fmt.Errorf("persistence: conversation compaction Run %q belongs to another session", planned.Expected.ID())
			}
			if planned.Expected.ID() != planned.Replacement.ID() {
				return fmt.Errorf("persistence: conversation compaction changes Run %q identity", planned.Expected.ID())
			}
		}
		if err := c.history.Replace(ctx, plan.SessionID, plan.Compaction.Messages()...); err != nil {
			return err
		}
		for _, planned := range plan.Runs {
			if planned.Expected.Equal(planned.Replacement) {
				continue
			}
			if err := c.runs.RebaseMessageMark(ctx, planned.Expected, planned.Replacement); err != nil {
				return err
			}
		}
		return nil
	})
}
