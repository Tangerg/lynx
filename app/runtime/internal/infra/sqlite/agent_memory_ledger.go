package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

var errAgentMemoryProject = errors.New("sqlite: agent memory project is required")

// AppendLedger inserts facts that have not already appeared in project. Facts
// are immutable and deduplicated independently, so a response containing one
// repeated bullet never suppresses its genuinely new siblings.
func (s *AgentMemoryStore) AppendLedger(ctx context.Context, batch agentmemory.FactBatch) ([]agentmemory.LedgerFact, error) {
	normalized, err := batch.Normalize()
	if err != nil {
		return nil, err
	}
	if len(normalized.Facts) == 0 {
		return nil, nil
	}
	var inserted []agentmemory.LedgerFact
	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		for _, fact := range normalized.Facts {
			result, err := conn(ctx, s.db).ExecContext(ctx,
				`INSERT OR IGNORE INTO agent_memory_ledger(
					project, day, session_id, fact, digest, captured_at
				) VALUES (?, ?, ?, ?, ?, ?)`,
				normalized.Project,
				normalized.Day,
				normalized.SessionID,
				fact,
				agentmemory.Digest(fact),
				normalized.CapturedAt.UTC().UnixNano(),
			)
			if err != nil {
				return fmt.Errorf("sqlite: append agent memory ledger: %w", err)
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("sqlite: inspect agent memory append: %w", err)
			}
			if changed == 0 {
				continue
			}
			sequence, err := result.LastInsertId()
			if err != nil {
				return fmt.Errorf("sqlite: read agent memory sequence: %w", err)
			}
			inserted = append(inserted, agentmemory.LedgerFact{
				Sequence:   sequence,
				Day:        normalized.Day,
				Content:    fact,
				CapturedAt: normalized.CapturedAt.UTC(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inserted, nil
}

// PendingLedger lists a project's facts strictly after watermark in sequence
// order. limit must be positive so every curation call has an explicit bound.
func (s *AgentMemoryStore) PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]agentmemory.LedgerFact, error) {
	if project == "" {
		return nil, errAgentMemoryProject
	}
	if watermark < 0 {
		return nil, errors.New("sqlite: agent memory watermark must not be negative")
	}
	if limit <= 0 {
		return nil, errors.New("sqlite: agent memory pending limit must be positive")
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT seq, day, fact, captured_at
		 FROM agent_memory_ledger
		 WHERE project = ? AND seq > ?
		 ORDER BY seq
		 LIMIT ?`, project, watermark, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list pending agent memory: %w", err)
	}
	defer rows.Close()
	var facts []agentmemory.LedgerFact
	for rows.Next() {
		var fact agentmemory.LedgerFact
		var capturedAt int64
		if err := rows.Scan(&fact.Sequence, &fact.Day, &fact.Content, &capturedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan pending agent memory: %w", err)
		}
		fact.CapturedAt = time.Unix(0, capturedAt).UTC()
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate pending agent memory: %w", err)
	}
	return facts, nil
}

// State returns the project's curation watermark. An unknown project has a zero
// watermark.
func (s *AgentMemoryStore) State(ctx context.Context, project string) (agentmemory.State, error) {
	if project == "" {
		return agentmemory.State{}, errAgentMemoryProject
	}
	var st agentmemory.State
	var updatedAt int64
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT watermark, updated_at FROM agent_memory_state WHERE project = ?`, project).
		Scan(&st.Watermark, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.State{}, nil
	}
	if err != nil {
		return agentmemory.State{}, fmt.Errorf("sqlite: load agent memory state: %w", err)
	}
	if updatedAt != 0 {
		st.UpdatedAt = time.Unix(0, updatedAt).UTC()
	}
	return st, nil
}

// Reconcile folds the project's ledger through `through` into its auto-origin
// item set. The watermark advance is a compare-and-swap around the LLM curation
// call: a concurrent winner returns published=false and leaves the item set
// untouched, so a lost race never half-applies a stale generation.
func (s *AgentMemoryStore) Reconcile(
	ctx context.Context,
	project string,
	expectedWatermark int64,
	through int64,
	contents []string,
	now time.Time,
) (published bool, err error) {
	if project == "" {
		return false, errAgentMemoryProject
	}
	if expectedWatermark < 0 || through <= expectedWatermark {
		return false, fmt.Errorf("sqlite: invalid agent memory watermark transition %d -> %d", expectedWatermark, through)
	}
	if now.IsZero() {
		return false, errors.New("sqlite: agent memory update time is required")
	}
	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		var one int
		if err := conn(ctx, s.db).QueryRowContext(ctx,
			`SELECT 1 FROM agent_memory_ledger WHERE project = ? AND seq = ?`,
			project, through).Scan(&one); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("sqlite: agent memory watermark %d does not belong to project", through)
			}
			return fmt.Errorf("sqlite: verify agent memory watermark: %w", err)
		}
		if _, err := conn(ctx, s.db).ExecContext(ctx,
			`INSERT OR IGNORE INTO agent_memory_state(project, watermark, updated_at) VALUES (?, 0, 0)`,
			project); err != nil {
			return fmt.Errorf("sqlite: initialize agent memory state: %w", err)
		}
		result, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE agent_memory_state SET watermark = ?, updated_at = ?
			 WHERE project = ? AND watermark = ?`,
			through, now.UTC().UnixNano(), project, expectedWatermark)
		if err != nil {
			return fmt.Errorf("sqlite: advance agent memory watermark: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect agent memory watermark: %w", err)
		}
		if changed != 1 {
			published = false
			return nil // lost CAS — do not reconcile items against a stale fold
		}
		if err := s.reconcileItems(ctx, project, contents, now); err != nil {
			return err
		}
		published = true
		return nil
	})
	return published, err
}
