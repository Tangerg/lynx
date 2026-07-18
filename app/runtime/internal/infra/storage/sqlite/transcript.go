package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type TranscriptStore struct{ db *sql.DB }

func NewTranscriptStore(db *sql.DB) *TranscriptStore { return &TranscriptStore{db: db} }

func (s *TranscriptStore) AppendItem(ctx context.Context, item transcript.Item) error {
	if item.SessionID == "" {
		return errors.New("sqlite: history item sessionId is required")
	}
	if item.ID == "" {
		return errors.New("sqlite: history item id is required")
	}
	var offloadID offload.ID
	if item.Tool != nil && item.Tool.Offload != nil {
		if err := item.Tool.Offload.Validate(); err != nil {
			return fmt.Errorf("sqlite: history item offload: %w", err)
		}
		if _, ok := item.Tool.Result.(string); !ok {
			return errors.New("sqlite: offloaded history item result must be a preview string")
		}
		offloadID = item.Tool.Offload.ID
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("sqlite: encode history item: %w", err)
	}
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO history_items(session_id, run_id, item_id, created_at, payload, offload_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET
		   payload = excluded.payload,
		   offload_id = excluded.offload_id
		 WHERE history_items.session_id = excluded.session_id
		   AND history_items.run_id = excluded.run_id
		   AND (history_items.offload_id = '' OR history_items.offload_id = excluded.offload_id)`,
		item.SessionID, item.RunID, item.ID, item.CreatedAt.UnixNano(), string(payload), offloadID,
	)
	if err != nil {
		if offloadID != "" {
			var ownerItem string
			ownerErr := conn(ctx, s.db).QueryRowContext(ctx,
				`SELECT item_id FROM history_items WHERE offload_id = ?`, offloadID,
			).Scan(&ownerItem)
			if ownerErr == nil && ownerItem != item.ID {
				return fmt.Errorf("%w: offload %q already belongs to item %q", transcript.ErrIdentityConflict, offloadID, ownerItem)
			}
			if ownerErr != nil && !errors.Is(ownerErr, sql.ErrNoRows) {
				return fmt.Errorf("sqlite: inspect history item offload conflict: %w", errors.Join(err, ownerErr))
			}
		}
		return fmt.Errorf("sqlite: append history item: %w", err)
	}
	if changed, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("sqlite: inspect history item write: %w", err)
	} else if changed != 1 {
		return fmt.Errorf("%w: item %q already belongs to another session, run, or offload identity", transcript.ErrIdentityConflict, item.ID)
	}
	return nil
}

func (s *TranscriptStore) PutRun(ctx context.Context, run transcript.Run) error {
	if run.SessionID == "" || run.ID == "" {
		return errors.New("sqlite: history run sessionId/id are required")
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("sqlite: encode history run: %w", err)
	}
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO history_runs(run_id, session_id, updated_at, payload, message_mark)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   updated_at   = excluded.updated_at,
		   payload      = excluded.payload,
		   message_mark = excluded.message_mark
		 WHERE history_runs.session_id = excluded.session_id`,
		run.ID, run.SessionID, run.UpdatedAt.UnixNano(), string(payload), run.MessageMark,
	)
	if err != nil {
		return fmt.Errorf("sqlite: put history run: %w", err)
	}
	if changed, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("sqlite: inspect history run write: %w", err)
	} else if changed != 1 {
		return fmt.Errorf("%w: run %q already belongs to another session", transcript.ErrIdentityConflict, run.ID)
	}
	return nil
}

func (s *TranscriptStore) DeleteRun(ctx context.Context, sessionID, runID string) error {
	if sessionID == "" || runID == "" {
		return errors.New("sqlite: delete history run requires sessionId + runId")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM tool_result_blobs
			 WHERE item_id IN (
			   SELECT item_id FROM history_items WHERE session_id = ? AND run_id = ?
			 )`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run tool results: %w", err)
		}
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_items WHERE session_id = ? AND run_id = ?`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run items: %w", err)
		}
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_runs WHERE run_id = ? AND session_id = ?`, runID, sessionID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run: %w", err)
		}
		return nil
	})
}

func (s *TranscriptStore) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("sqlite: delete history session requires sessionId")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		if _, err := q.ExecContext(ctx, `DELETE FROM history_items WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session items: %w", err)
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM history_runs WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session runs: %w", err)
		}
		return nil
	})
}

func (s *TranscriptStore) List(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	items, err := s.listItems(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	runs, err := s.listRuns(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	return items, runs, nil
}

func (s *TranscriptStore) ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	return s.listRuns(ctx, sessionID)
}

func (s *TranscriptStore) listItems(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT h.session_id, h.run_id, h.item_id, h.created_at, h.payload, h.offload_id, b.body
		 FROM history_items AS h
		 LEFT JOIN tool_result_blobs AS b
		   ON b.id = h.offload_id AND b.session_id = h.session_id AND b.item_id = h.item_id
		 WHERE h.session_id = ? ORDER BY h.seq`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	defer rows.Close()

	var out []transcript.Item
	for rows.Next() {
		var session, runID, itemID, payload, rawOffloadID string
		var offloadedBody sql.NullString
		var createdAt int64
		if err := rows.Scan(&session, &runID, &itemID, &createdAt, &payload, &rawOffloadID, &offloadedBody); err != nil {
			return nil, fmt.Errorf("sqlite: scan history item: %w", err)
		}
		var item transcript.Item
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			return nil, fmt.Errorf("sqlite: decode history item %q: %w", itemID, err)
		}
		item.SessionID = session
		item.RunID = runID
		item.ID = itemID
		item.CreatedAt = time.Unix(0, createdAt).UTC()
		if rawOffloadID != "" {
			id, err := offload.ParseID(rawOffloadID)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode history item %q offload: %w", itemID, err)
			}
			if item.Tool == nil {
				return nil, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", itemID)
			}
			if _, ok := item.Tool.Result.(string); !ok {
				return nil, fmt.Errorf("sqlite: history item %q has an offload identity but no preview string", itemID)
			}
			if !offloadedBody.Valid {
				return nil, fmt.Errorf("sqlite: history item %q references missing tool result %q", itemID, id)
			}
			item.Tool.Offload = &offload.Ref{ID: id}
			item.Tool.Result = offloadedBody.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	return out, nil
}

func (s *TranscriptStore) listRuns(ctx context.Context, sessionID string) ([]transcript.Run, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT session_id, run_id, updated_at, payload, message_mark
		 FROM history_runs WHERE session_id = ? ORDER BY updated_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history runs: %w", err)
	}
	defer rows.Close()

	var out []transcript.Run
	for rows.Next() {
		var session, runID, payload string
		var updatedAt int64
		var mark int
		if err := rows.Scan(&session, &runID, &updatedAt, &payload, &mark); err != nil {
			return nil, fmt.Errorf("sqlite: scan history run: %w", err)
		}
		var run transcript.Run
		if err := json.Unmarshal([]byte(payload), &run); err != nil {
			return nil, fmt.Errorf("sqlite: decode history run %q: %w", runID, err)
		}
		run.SessionID = session
		run.ID = runID
		run.UpdatedAt = time.Unix(0, updatedAt).UTC()
		run.MessageMark = mark
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list history runs: %w", err)
	}
	return out, nil
}
