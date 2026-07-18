package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// AgentMemoryStore persists append-only extracted facts and the atomic curated
// projection built from them.
type AgentMemoryStore struct {
	db *sql.DB
}

// NewAgentMemoryStore binds a database opened by Open.
func NewAgentMemoryStore(db *sql.DB) *AgentMemoryStore {
	return &AgentMemoryStore{db: db}
}

// AppendLedger inserts facts that have not already appeared in project. Facts
// are immutable and deduplicated independently, so a response containing one
// repeated bullet never suppresses its genuinely new siblings.
func (s *AgentMemoryStore) AppendLedger(ctx context.Context, batch knowledge.FactBatch) ([]knowledge.LedgerFact, error) {
	normalized, err := batch.Normalize()
	if err != nil {
		return nil, err
	}
	if len(normalized.Facts) == 0 {
		return nil, nil
	}
	var inserted []knowledge.LedgerFact
	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		for _, fact := range normalized.Facts {
			digest := sha256.Sum256([]byte(fact))
			result, err := conn(ctx, s.db).ExecContext(ctx,
				`INSERT OR IGNORE INTO agent_memory_ledger(
					project, day, session_id, fact, digest, captured_at
				) VALUES (?, ?, ?, ?, ?, ?)`,
				normalized.Project,
				normalized.Day,
				normalized.SessionID,
				fact,
				hex.EncodeToString(digest[:]),
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
			inserted = append(inserted, knowledge.LedgerFact{
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
func (s *AgentMemoryStore) PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]knowledge.LedgerFact, error) {
	if project == "" {
		return nil, errors.New("sqlite: agent memory project is required")
	}
	if watermark < 0 {
		return nil, errors.New("sqlite: agent memory watermark must not be negative")
	}
	if limit <= 0 {
		return nil, errors.New("sqlite: agent memory pending limit must be positive")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, day, fact, captured_at
		 FROM agent_memory_ledger
		 WHERE project = ? AND seq > ?
		 ORDER BY seq
		 LIMIT ?`, project, watermark, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list pending agent memory: %w", err)
	}
	defer rows.Close()
	var facts []knowledge.LedgerFact
	for rows.Next() {
		var fact knowledge.LedgerFact
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

// CuratedMemory returns the currently published complete projection. An
// unknown project has empty content and watermark zero.
func (s *AgentMemoryStore) CuratedMemory(ctx context.Context, project string) (knowledge.Curated, error) {
	if project == "" {
		return knowledge.Curated{}, errors.New("sqlite: agent memory project is required")
	}
	var memory knowledge.Curated
	var updatedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT content, watermark, updated_at
		 FROM agent_memory_curated
		 WHERE project = ?`, project).Scan(&memory.Content, &memory.Watermark, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.Curated{}, nil
	}
	if err != nil {
		return knowledge.Curated{}, fmt.Errorf("sqlite: load curated agent memory: %w", err)
	}
	if updatedAt != 0 {
		memory.UpdatedAt = time.Unix(0, updatedAt).UTC()
	}
	return memory, nil
}

// PublishCuratedMemory atomically replaces the complete curated body and
// advances its watermark. expectedWatermark is a compare-and-swap guard around
// the LLM call; a concurrent winner returns published=false without clobbering
// fresher memory.
func (s *AgentMemoryStore) PublishCuratedMemory(
	ctx context.Context,
	project string,
	expectedWatermark int64,
	through int64,
	content string,
	updatedAt time.Time,
) (published bool, err error) {
	if project == "" {
		return false, errors.New("sqlite: agent memory project is required")
	}
	if expectedWatermark < 0 || through <= expectedWatermark {
		return false, fmt.Errorf("sqlite: invalid agent memory watermark transition %d -> %d", expectedWatermark, through)
	}
	if updatedAt.IsZero() {
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
			`INSERT OR IGNORE INTO agent_memory_curated(project, content, watermark, updated_at)
			 VALUES (?, '', 0, 0)`, project); err != nil {
			return fmt.Errorf("sqlite: initialize curated agent memory: %w", err)
		}
		result, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE agent_memory_curated
			 SET content = ?, watermark = ?, updated_at = ?
			 WHERE project = ? AND watermark = ?`,
			content, through, updatedAt.UTC().UnixNano(), project, expectedWatermark)
		if err != nil {
			return fmt.Errorf("sqlite: publish curated agent memory: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect curated agent memory publish: %w", err)
		}
		published = changed == 1
		return nil
	})
	return published, err
}
