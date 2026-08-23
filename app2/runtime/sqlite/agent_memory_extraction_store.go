package sqlite

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/agentmemory"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
)

const (
	maxAgentMemoryMaintenanceRuns = 64
	maxAgentMemoryRunMessages     = 512
)

func (database *Database) ListAgentMemoryMaintenanceRuns(
	ctx context.Context,
	attemptedBefore time.Time,
	limit int,
) ([]agentmemory.MaintenanceRun, error) {
	attemptedBefore = attemptedBefore.UTC()
	if attemptedBefore.IsZero() ||
		limit <= 0 || limit > maxAgentMemoryMaintenanceRuns {
		return nil, errors.New("sqlite: invalid AgentMemory maintenance Run limit")
	}
	rows, err := database.database.QueryContext(ctx, `
		SELECT r.id, r.session_id, s.workspace_path, r.provider, r.model,
			r.finished_at
		FROM runs r
		JOIN sessions s ON s.id = r.session_id
		LEFT JOIN agent_memory_extractions e ON e.run_id = r.id
		LEFT JOIN agent_memory_extraction_attempts attempt ON attempt.run_id = r.id
		WHERE r.parent_run_id IS NULL
			AND r.status = 'finished'
			AND r.outcome = 'completed'
			AND e.run_id IS NULL
			AND (
				attempt.run_id IS NULL
				OR julianday(attempt.attempted_at) < julianday(?)
			)
		ORDER BY attempt.attempted_at IS NOT NULL, attempt.attempted_at,
			r.finished_at, r.id
		LIMIT ?`,
		encodeTime(attemptedBefore),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list AgentMemory maintenance Runs: %w", err)
	}
	defer rows.Close()
	values := make([]agentmemory.MaintenanceRun, 0)
	for rows.Next() {
		var value agentmemory.MaintenanceRun
		var finishedAt string
		if err := rows.Scan(
			&value.RunID,
			&value.SessionID,
			&value.Workspace,
			&value.Selection.Provider,
			&value.Selection.Model,
			&finishedAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan AgentMemory maintenance Run: %w", err)
		}
		value.FinishedAt, err = decodeTime(finishedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode AgentMemory Run finish: %w", err)
		}
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf(
				"sqlite: invalid AgentMemory maintenance Run %q: %w",
				value.RunID,
				err,
			)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate AgentMemory maintenance Runs: %w", err)
	}
	return values, nil
}

func (database *Database) MarkAgentMemoryExtractionAttempt(
	ctx context.Context,
	runID string,
	attemptedAt time.Time,
) error {
	attemptedAt = attemptedAt.UTC()
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(runID) != runID ||
		attemptedAt.IsZero() {
		return errors.New("sqlite: invalid AgentMemory extraction attempt")
	}
	if _, err := database.database.ExecContext(ctx, `
		INSERT INTO agent_memory_extraction_attempts (run_id, attempted_at)
		SELECT ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM agent_memory_extractions WHERE run_id = ?
		)
		ON CONFLICT(run_id) DO UPDATE SET attempted_at = excluded.attempted_at`,
		runID,
		encodeTime(attemptedAt),
		runID,
	); err != nil {
		return fmt.Errorf("sqlite: mark AgentMemory extraction attempt: %w", err)
	}
	return nil
}

func (database *Database) ListAgentMemoryRunMessages(
	ctx context.Context,
	runID string,
) ([]conversationdomain.Record, error) {
	values := make(map[int]conversationdomain.Record, maxAgentMemoryRunMessages)
	read := func(query string, arguments ...any) error {
		rows, err := database.database.QueryContext(ctx, query, arguments...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value conversationdomain.Record
			if err := rows.Scan(
				&value.SessionID,
				&value.RunID,
				&value.Ordinal,
				&value.Body,
			); err != nil {
				return err
			}
			values[value.Ordinal] = value
		}
		return rows.Err()
	}
	if err := read(`
		SELECT session_id, run_id, ordinal, body
		FROM conversation_messages
		WHERE run_id = ?
		ORDER BY ordinal
		LIMIT 1`, runID); err != nil {
		return nil, fmt.Errorf("sqlite: read first AgentMemory Run message: %w", err)
	}
	if err := read(`
		SELECT session_id, run_id, ordinal, body
		FROM conversation_messages
		WHERE run_id = ?
		ORDER BY ordinal DESC
		LIMIT ?`, runID, maxAgentMemoryRunMessages-1); err != nil {
		return nil, fmt.Errorf("sqlite: read recent AgentMemory Run messages: %w", err)
	}
	result := make([]conversationdomain.Record, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	slices.SortFunc(result, func(left, right conversationdomain.Record) int {
		return cmp.Compare(left.Ordinal, right.Ordinal)
	})
	return result, nil
}

func (database *Database) CommitAgentMemoryExtraction(
	ctx context.Context,
	input agentmemory.FactBatch,
) error {
	batch, err := input.Normalize()
	if err != nil {
		return err
	}
	_, err = runAgentMemoryTx(ctx, database, func(tx *sql.Tx) (bool, error) {
		var sessionID string
		var provider string
		var model string
		var parent sql.NullString
		var status string
		var outcome sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT session_id, provider, model, parent_run_id, status, outcome
			FROM runs
			WHERE id = ?`, batch.RunID).Scan(
			&sessionID,
			&provider,
			&model,
			&parent,
			&status,
			&outcome,
		); errors.Is(err, sql.ErrNoRows) {
			return false, agentmemory.ErrNotFound
		} else if err != nil {
			return false, fmt.Errorf("sqlite: inspect AgentMemory source Run: %w", err)
		}
		if sessionID != batch.SessionID || provider != batch.Source.Provider ||
			model != batch.Source.Model || parent.Valid || status != "finished" ||
			!outcome.Valid || outcome.String != "completed" {
			return false, errors.New(
				"sqlite: AgentMemory extraction source is not the committed completed root Run",
			)
		}
		var extractorProvider any
		var extractorModel any
		if batch.Extractor != nil {
			extractorProvider = batch.Extractor.Provider
			extractorModel = batch.Extractor.Model
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO agent_memory_extractions (
				run_id, session_id, project_path, day,
				extractor_provider, extractor_model, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(run_id) DO NOTHING`,
			batch.RunID,
			batch.SessionID,
			batch.Project,
			batch.Day,
			extractorProvider,
			extractorModel,
			encodeTime(batch.CapturedAt),
		)
		if err != nil {
			return false, fmt.Errorf("sqlite: commit AgentMemory extraction receipt: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("sqlite: inspect AgentMemory extraction receipt: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM agent_memory_extraction_attempts WHERE run_id = ?`,
			batch.RunID,
		); err != nil {
			return false, fmt.Errorf(
				"sqlite: clear AgentMemory extraction attempt: %w",
				err,
			)
		}
		if inserted == 0 {
			return false, nil
		}
		for _, fact := range batch.Facts {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO agent_memory_ledger (
					run_id, content, digest
				) VALUES (?, ?, ?)`,
				batch.RunID,
				fact,
				agentmemory.Digest(fact),
			); err != nil {
				return false, fmt.Errorf("sqlite: append AgentMemory ledger fact: %w", err)
			}
		}
		return true, nil
	})
	return err
}
