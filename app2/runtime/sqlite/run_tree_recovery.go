package sqlite

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

// CommitTreeRecovery atomically marks one predecessor-owned open tree lost.
// Members and events are committed descendant-first so the root closes the
// carrying stream only after every child and parent Delegate Item is durable.
func (database *Database) CommitTreeRecovery(
	ctx context.Context,
	write runflow.TreeRecoveryWrite,
) error {
	if len(write.Runs) == 0 {
		return errors.New("sqlite: tree recovery is empty")
	}
	root := write.Runs[len(write.Runs)-1]
	rootRunID := root.Run.Run.ID()
	rootSegmentID := root.ExpectedSegmentID
	if root.Depth != 0 || root.Run.Run.ParentRunID() != "" ||
		root.Run.Run.Status() != rundomain.Finished || root.Run.Run.Outcome() != rundomain.Lost ||
		rootSegmentID == "" {
		return errors.New("sqlite: tree recovery root is invalid")
	}
	lastDepth := ^uint32(0)
	planned := make(map[string]bool, len(write.Runs))
	for _, member := range write.Runs {
		value := member.Run.Run
		if member.Depth > lastDepth || planned[value.ID()] || value.Status() != rundomain.Finished ||
			value.Outcome() != rundomain.Lost || member.ExpectedSegmentID == "" ||
			value.SessionID() != root.Run.Run.SessionID() {
			return errors.New("sqlite: tree recovery members are invalid or out of order")
		}
		lastDepth = member.Depth
		planned[value.ID()] = true
	}
	for _, member := range write.Runs {
		value := member.Run.Run
		if value.ID() == rootRunID {
			if value.RootRunID() != "" || value.ParentRunID() != "" {
				return errors.New("sqlite: tree recovery root changed lineage")
			}
		} else if value.RootRunID() != rootRunID || value.ParentRunID() == "" ||
			!planned[value.ParentRunID()] {
			return errors.New("sqlite: tree recovery child changed lineage")
		}
		for _, event := range member.Events {
			if event.RootRunID != rootRunID || event.RootSegmentID != rootSegmentID ||
				event.RunID != value.ID() || event.SegmentID != member.ExpectedSegmentID {
				return errors.New("sqlite: tree recovery event changed stream ownership")
			}
		}
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tree recovery: %w", err)
	}
	defer transaction.Rollback()
	var runningMembers int
	if err := transaction.QueryRowContext(ctx, `
		SELECT count(*) FROM runs
		WHERE status = 'running' AND (id = ? OR root_run_id = ?)`,
		rootRunID, rootRunID,
	).Scan(&runningMembers); err != nil {
		return fmt.Errorf("sqlite: count recovered tree members: %w", err)
	}
	if runningMembers != len(write.Runs) {
		return rundomain.ErrInvalidTransition
	}
	for _, member := range write.Runs {
		value := member.Run.Run
		var currentSessionID, currentParentRunID, currentRootRunID string
		if err := transaction.QueryRowContext(ctx, `
			SELECT session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, '')
			FROM runs WHERE id = ?`, value.ID(),
		).Scan(&currentSessionID, &currentParentRunID, &currentRootRunID); err != nil {
			return fmt.Errorf("sqlite: read recovery Run %s lineage: %w", value.ID(), err)
		}
		if currentSessionID != value.SessionID() || currentParentRunID != value.ParentRunID() ||
			currentRootRunID != value.RootRunID() {
			return errors.New("sqlite: recovery durable lineage changed")
		}
		result, err := transaction.ExecContext(ctx, `
			UPDATE runs SET status = 'finished', active_segment_id = NULL,
				outcome = ?, detail = ?, body = ?, updated_at = ?, finished_at = ?
			WHERE id = ? AND status = 'running' AND active_segment_id = ?`,
			string(value.Outcome()), value.Detail(), string(member.Run.Body),
			encodeTime(value.UpdatedAt()), encodeTime(value.FinishedAt()),
			value.ID(), member.ExpectedSegmentID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: recover Run %s: %w", value.ID(), err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return rundomain.ErrInvalidTransition
		}
		for _, item := range member.Items {
			if item.RunID != value.ID() || item.SessionID != value.SessionID() {
				return errors.New("sqlite: recovered Item changed Run ownership")
			}
			if err := putItem(ctx, transaction, item); err != nil {
				return err
			}
		}
		if err := insertRunEvents(ctx, transaction, member.Events); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id = ?`, value.ID()); err != nil {
			return fmt.Errorf("sqlite: clear recovered interrupt: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `DELETE FROM executor_checkpoints WHERE run_id = ?`, value.ID()); err != nil {
			return fmt.Errorf("sqlite: clear recovered checkpoint: %w", err)
		}
	}
	for _, message := range write.Messages {
		if message.RunID != rootRunID || message.SessionID != root.Run.Run.SessionID() {
			return errors.New("sqlite: recovered Conversation result changed root ownership")
		}
		if err := insertConversationMessage(ctx, transaction, message); err != nil {
			return err
		}
	}
	if err := capturePlanBoundary(ctx, transaction, rootRunID, root.Run.Run.SessionID()); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit tree recovery: %w", err)
	}
	return nil
}
