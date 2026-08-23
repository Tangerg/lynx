package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/agentmemory"
)

const maxAgentMemoryProjects = 64

func (database *Database) ListAgentMemoryCurationProjects(
	ctx context.Context,
	minimumFacts int,
	staleBefore time.Time,
	limit int,
) ([]string, error) {
	staleBefore = staleBefore.UTC()
	if minimumFacts <= 0 || staleBefore.IsZero() ||
		limit <= 0 || limit > maxAgentMemoryProjects {
		return nil, errors.New("sqlite: invalid AgentMemory project limit")
	}
	rows, err := database.database.QueryContext(ctx, `
		SELECT extraction.project_path
		FROM agent_memory_ledger ledger
		JOIN agent_memory_extractions extraction
			ON extraction.run_id = ledger.run_id
		LEFT JOIN agent_memory_curation state
			ON state.project_path = extraction.project_path
		WHERE ledger.sequence > coalesce(state.watermark, 0)
		GROUP BY extraction.project_path, state.project_path, state.updated_at
		HAVING state.project_path IS NULL
			OR count(*) >= ?
			OR julianday(state.updated_at) <= julianday(?)
		ORDER BY min(ledger.sequence), extraction.project_path
		LIMIT ?`,
		minimumFacts,
		encodeTime(staleBefore),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list AgentMemory curation projects: %w", err)
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var project string
		if err := rows.Scan(&project); err != nil {
			return nil, fmt.Errorf("sqlite: scan AgentMemory curation project: %w", err)
		}
		values = append(values, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate AgentMemory curation projects: %w", err)
	}
	return values, nil
}

func (database *Database) GetAgentMemoryCurationState(
	ctx context.Context,
	project string,
) (agentmemory.CurationState, error) {
	if !filepath.IsAbs(project) || filepath.Clean(project) != project {
		return agentmemory.CurationState{}, errors.New(
			"sqlite: AgentMemory curation project must be canonical and absolute",
		)
	}
	var state agentmemory.CurationState
	var updatedAt string
	err := database.database.QueryRowContext(ctx, `
		SELECT watermark, revision, updated_at
		FROM agent_memory_curation
		WHERE project_path = ?`, project).Scan(
		&state.Watermark,
		&state.Revision,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("sqlite: get AgentMemory curation state: %w", err)
	}
	state.UpdatedAt, err = decodeTime(updatedAt)
	if err != nil {
		return agentmemory.CurationState{}, fmt.Errorf(
			"sqlite: decode AgentMemory curation time: %w",
			err,
		)
	}
	if err := state.Validate(); err != nil {
		return agentmemory.CurationState{}, fmt.Errorf(
			"sqlite: invalid AgentMemory curation state: %w",
			err,
		)
	}
	return state, nil
}

func (database *Database) ListAgentMemoryLedger(
	ctx context.Context,
	project string,
	after int64,
	limit int,
) ([]agentmemory.LedgerFact, error) {
	if !filepath.IsAbs(project) || filepath.Clean(project) != project ||
		limit <= 0 || limit > agentmemory.MaxLedgerFoldFacts || after < 0 {
		return nil, errors.New("sqlite: invalid AgentMemory ledger query")
	}
	rows, err := database.database.QueryContext(ctx, `
		SELECT ledger.sequence, extraction.run_id, extraction.session_id,
			extraction.day, ledger.content, extraction.extractor_provider,
			extraction.extractor_model, extraction.completed_at
		FROM agent_memory_ledger ledger
		JOIN agent_memory_extractions extraction
			ON extraction.run_id = ledger.run_id
		WHERE extraction.project_path = ? AND ledger.sequence > ?
		ORDER BY ledger.sequence
		LIMIT ?`,
		project,
		after,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list AgentMemory ledger: %w", err)
	}
	defer rows.Close()
	values := make([]agentmemory.LedgerFact, 0)
	for rows.Next() {
		var value agentmemory.LedgerFact
		var capturedAt string
		var provider sql.NullString
		var model sql.NullString
		if err := rows.Scan(
			&value.Sequence,
			&value.RunID,
			&value.SessionID,
			&value.Day,
			&value.Content,
			&provider,
			&model,
			&capturedAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan AgentMemory ledger: %w", err)
		}
		if provider.Valid {
			value.Selection.Provider = provider.String
		}
		if model.Valid {
			value.Selection.Model = model.String
		}
		value.CapturedAt, err = decodeTime(capturedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode AgentMemory ledger time: %w", err)
		}
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf(
				"sqlite: invalid AgentMemory ledger fact %d: %w",
				value.Sequence,
				err,
			)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate AgentMemory ledger: %w", err)
	}
	return values, nil
}

func (database *Database) ListAgentMemoryCurationItems(
	ctx context.Context,
	project string,
) ([]agentmemory.Item, error) {
	if !filepath.IsAbs(project) || filepath.Clean(project) != project {
		return nil, errors.New(
			"sqlite: AgentMemory curation project must be canonical and absolute",
		)
	}
	rows, err := database.database.QueryContext(ctx, `
		SELECT `+agentMemoryColumns+`
		FROM agent_memory
		WHERE (scope = 'project' AND project_path = ?)
			OR scope = 'user'
		ORDER BY
			CASE status WHEN 'active' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,
			pinned DESC,
			updated_at DESC,
			id
		LIMIT ?`,
		project,
		agentmemory.MaxCurationItems,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list AgentMemory curation context: %w", err)
	}
	defer rows.Close()
	values := make([]agentmemory.Item, 0)
	for rows.Next() {
		item, err := scanAgentMemory(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate AgentMemory curation context: %w", err)
	}
	return values, nil
}

func (database *Database) CommitAgentMemoryCuration(
	ctx context.Context,
	project string,
	expectedWatermark int64,
	through int64,
	proposals []agentmemory.Item,
	now time.Time,
) (bool, bool, error) {
	now = now.UTC()
	if !filepath.IsAbs(project) || filepath.Clean(project) != project ||
		now.IsZero() || expectedWatermark < 0 || through <= expectedWatermark ||
		len(proposals) > agentmemory.MaxFactsPerExtraction {
		return false, false, errors.New("sqlite: invalid AgentMemory curation commit")
	}
	for _, proposal := range proposals {
		if err := proposal.Validate(); err != nil {
			return false, false, err
		}
		if proposal.Scope != agentmemory.ScopeProject ||
			proposal.Project != project ||
			proposal.Origin != agentmemory.OriginAuto ||
			proposal.Status != agentmemory.StatusPending {
			return false, false, errors.New(
				"sqlite: AgentMemory curation proposal changed target or lifecycle",
			)
		}
	}
	type result struct {
		published bool
		won       bool
	}
	value, err := runAgentMemoryTx(ctx, database, func(tx *sql.Tx) (result, error) {
		var last int64
		if err := tx.QueryRowContext(ctx, `
			SELECT coalesce(max(sequence), 0)
			FROM agent_memory_ledger ledger
			JOIN agent_memory_extractions extraction
				ON extraction.run_id = ledger.run_id
			WHERE extraction.project_path = ?
				AND ledger.sequence > ? AND ledger.sequence <= ?`,
			project,
			expectedWatermark,
			through,
		).Scan(&last); err != nil {
			return result{}, fmt.Errorf("sqlite: verify AgentMemory curation range: %w", err)
		}
		if last != through {
			return result{}, errors.New("sqlite: AgentMemory curation range is stale")
		}

		var cas sql.Result
		var err error
		if expectedWatermark == 0 {
			cas, err = tx.ExecContext(ctx, `
				INSERT INTO agent_memory_curation (
					project_path, watermark, revision, updated_at
				) VALUES (?, ?, 1, ?)
				ON CONFLICT(project_path) DO NOTHING`,
				project,
				through,
				encodeTime(now),
			)
		} else {
			cas, err = tx.ExecContext(ctx, `
				UPDATE agent_memory_curation
				SET watermark = ?, revision = revision + 1, updated_at = ?
				WHERE project_path = ? AND watermark = ?`,
				through,
				encodeTime(now),
				project,
				expectedWatermark,
			)
		}
		if err != nil {
			return result{}, fmt.Errorf("sqlite: advance AgentMemory curation: %w", err)
		}
		won, err := cas.RowsAffected()
		if err != nil {
			return result{}, fmt.Errorf("sqlite: inspect AgentMemory curation CAS: %w", err)
		}
		if won == 0 {
			return result{}, nil
		}

		visible, err := countVisibleAgentMemory(
			ctx,
			tx,
			agentmemory.ScopeProject,
			project,
		)
		if err != nil {
			return result{}, err
		}
		published := false
		for _, proposal := range proposals {
			if visible >= agentmemory.MaxVisiblePerTarget {
				break
			}
			_, found, err := findAgentMemoryDigest(
				ctx,
				tx,
				proposal.Scope,
				proposal.Project,
				proposal.Digest,
			)
			if err != nil {
				return result{}, err
			}
			if found {
				continue
			}
			if err := insertAgentMemory(ctx, tx, proposal); err != nil {
				return result{}, err
			}
			visible++
			published = true
		}
		return result{published: published, won: true}, nil
	})
	if err != nil {
		return false, false, err
	}
	return value.published, value.won, nil
}
