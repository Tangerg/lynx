package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// AgentMemoryStore persists the append-only extracted fact ledger and the
// addressable memory items curated from it. The ledger is raw project-scoped
// capture; items are the curated projection reconciled from a ledger fold.
type AgentMemoryStore struct {
	db *sql.DB
}

var _ agentmemory.Store = (*AgentMemoryStore)(nil)

// NewAgentMemoryStore binds a database opened by Open.
func NewAgentMemoryStore(db *sql.DB) *AgentMemoryStore {
	return &AgentMemoryStore{db: db}
}

var errAgentMemoryProject = errors.New("sqlite: agent memory project is required")

// digestOf is the content-identity used both to deduplicate ledger facts and to
// match items across a reconcile so unchanged content keeps its stable id.
func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func newMemoryID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("sqlite: mint memory id: %w", err)
	}
	return "mem_" + hex.EncodeToString(buf[:]), nil
}

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
				digestOf(fact),
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
	err := s.db.QueryRowContext(ctx,
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

// reconcileItems replaces the project's auto-origin, unpinned items with the
// curated contents: it deletes those whose content is no longer present and
// inserts the genuinely new ones, matching by digest so unchanged items keep
// their id and provenance. Pinned and user-authored items are never touched.
func (s *AgentMemoryStore) reconcileItems(ctx context.Context, project string, contents []string, now time.Time) error {
	desired := make(map[string]string, len(contents))
	order := make([]string, 0, len(contents))
	for _, content := range contents {
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		digest := digestOf(content)
		if _, dup := desired[digest]; dup {
			continue
		}
		desired[digest] = content
		order = append(order, digest)
	}

	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT id, digest FROM agent_memory_items
		 WHERE scope = 'project' AND project = ? AND origin = 'auto' AND pinned = 0`, project)
	if err != nil {
		return fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	type existing struct{ id, digest string }
	var have []existing
	for rows.Next() {
		var e existing
		if err := rows.Scan(&e.id, &e.digest); err != nil {
			rows.Close()
			return fmt.Errorf("sqlite: scan agent memory item: %w", err)
		}
		have = append(have, e)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: close agent memory items: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}

	present := make(map[string]struct{}, len(have))
	for _, e := range have {
		if _, keep := desired[e.digest]; keep {
			present[e.digest] = struct{}{}
			continue
		}
		if _, err := conn(ctx, s.db).ExecContext(ctx,
			`DELETE FROM agent_memory_items WHERE id = ?`, e.id); err != nil {
			return fmt.Errorf("sqlite: prune agent memory item: %w", err)
		}
	}

	nano := now.UTC().UnixNano()
	day := now.UTC().Format(time.DateOnly)
	for _, digest := range order {
		if _, exists := present[digest]; exists {
			continue
		}
		id, err := newMemoryID()
		if err != nil {
			return err
		}
		// OR IGNORE: a pinned or user item may already hold this content under
		// the unique (scope, project, digest) index — keep it, don't duplicate.
		if _, err := conn(ctx, s.db).ExecContext(ctx,
			`INSERT OR IGNORE INTO agent_memory_items(
				id, scope, project, content, digest, origin, pinned, session_id, day, created_at, updated_at
			) VALUES (?, 'project', ?, ?, ?, 'auto', 0, '', ?, ?, ?)`,
			id, project, desired[digest], digest, day, nano, nano); err != nil {
			return fmt.Errorf("sqlite: insert agent memory item: %w", err)
		}
	}
	return nil
}

// Items lists the active items for (scope, project): pinned first, then most
// recently updated.
func (s *AgentMemoryStore) Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scope, project, content, origin, pinned, session_id, day, created_at, updated_at
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ?
		 ORDER BY pinned DESC, updated_at DESC`, scope.String(), project)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	defer rows.Close()
	var items []agentmemory.Item
	for rows.Next() {
		var (
			item                 agentmemory.Item
			scopeText, originText string
			pinned               int
			createdAt, updatedAt int64
		)
		if err := rows.Scan(&item.ID, &scopeText, &item.Project, &item.Content, &originText,
			&pinned, &item.SessionID, &item.Day, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", err)
		}
		item.Scope = agentmemory.ParseScope(scopeText)
		item.Origin = agentmemory.ParseOrigin(originText)
		item.Pinned = pinned != 0
		item.CreatedAt = time.Unix(0, createdAt).UTC()
		item.UpdatedAt = time.Unix(0, updatedAt).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
}
