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

// reconcileItems folds the curated contents into the project's auto-origin
// items as PENDING proposals for human review. A curated fact not yet
// represented (in any status) becomes a new pending item; a pending proposal the
// curator no longer produces is pruned. Approved (active) and declined
// (rejected) items are sticky: active memory the user accepted is never
// auto-removed, and a rejected tombstone blocks the same fact from being
// re-proposed. Pinned and user-authored items are never touched.
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
		`SELECT id, digest, status FROM agent_memory_items
		 WHERE scope = 'project' AND project = ? AND origin = 'auto' AND pinned = 0`, project)
	if err != nil {
		return fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	type existing struct{ id, digest, status string }
	var have []existing
	for rows.Next() {
		var e existing
		if err := rows.Scan(&e.id, &e.digest, &e.status); err != nil {
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
		present[e.digest] = struct{}{}
		// Prune only stale PENDING proposals; active/rejected are sticky.
		if e.status == "pending" {
			if _, keep := desired[e.digest]; !keep {
				if _, err := conn(ctx, s.db).ExecContext(ctx,
					`DELETE FROM agent_memory_items WHERE id = ?`, e.id); err != nil {
					return fmt.Errorf("sqlite: prune agent memory item: %w", err)
				}
			}
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
				id, scope, project, content, digest, origin, status, pinned, session_id, day, created_at, updated_at
			) VALUES (?, 'project', ?, ?, ?, 'auto', 'pending', 0, '', ?, ?, ?)`,
			id, project, desired[digest], digest, day, nano, nano); err != nil {
			return fmt.Errorf("sqlite: insert agent memory item: %w", err)
		}
	}
	return nil
}

const agentMemoryItemColumns = `id, scope, project, content, origin, status, pinned, session_id, day, created_at, updated_at`

// scanItem decodes one item's base columns (embedding excluded — see
// [AgentMemoryStore.ItemsForSearch] for the search path that reads it).
func scanItem(row scanRow) (agentmemory.Item, error) {
	var (
		item                              agentmemory.Item
		scopeText, originText, statusText string
		pinned                            int
		createdAt, updatedAt              int64
	)
	if err := row.Scan(&item.ID, &scopeText, &item.Project, &item.Content, &originText, &statusText,
		&pinned, &item.SessionID, &item.Day, &createdAt, &updatedAt); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: scan agent memory item: %w", err)
	}
	item.Scope = agentmemory.ParseScope(scopeText)
	item.Origin = agentmemory.ParseOrigin(originText)
	item.Status = agentmemory.ParseStatus(statusText)
	item.Pinned = pinned != 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return item, nil
}

// Items lists the active items for (scope, project): pinned first, then most
// recently updated. Pending and rejected items are excluded — only approved
// memory is injected into the prompt.
func (s *AgentMemoryStore) Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active'
		 ORDER BY pinned DESC, updated_at DESC`, scope.String(), project)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	defer rows.Close()
	var items []agentmemory.Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
}

// ItemsForSearch lists the active (scope, project) items with their embedding
// decoded, for in-process keyword + vector ranking. Only approved memory is
// searchable.
func (s *AgentMemoryStore) ItemsForSearch(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+`, embedding
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active'`, scope.String(), project)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items for search: %w", err)
	}
	defer rows.Close()
	var items []agentmemory.Item
	for rows.Next() {
		var (
			item                              agentmemory.Item
			scopeText, originText, statusText string
			pinned                            int
			createdAt, updatedAt              int64
			blob                              []byte
		)
		if err := rows.Scan(&item.ID, &scopeText, &item.Project, &item.Content, &originText, &statusText,
			&pinned, &item.SessionID, &item.Day, &createdAt, &updatedAt, &blob); err != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", err)
		}
		item.Scope = agentmemory.ParseScope(scopeText)
		item.Origin = agentmemory.ParseOrigin(originText)
		item.Status = agentmemory.ParseStatus(statusText)
		item.Pinned = pinned != 0
		item.CreatedAt = time.Unix(0, createdAt).UTC()
		item.UpdatedAt = time.Unix(0, updatedAt).UTC()
		if len(blob) > 0 {
			item.Embedding = decodeVec(blob)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
}

// UnembeddedItems lists the active (scope, project) items that still lack an
// embedding — the searchable set an embedder should backfill.
func (s *AgentMemoryStore) UnembeddedItems(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active' AND length(embedding) = 0`, scope.String(), project)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list unembedded agent memory items: %w", err)
	}
	defer rows.Close()
	var items []agentmemory.Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
}

// List returns the (scope, project) items the review surface shows: active and
// pending, ordered pending-first (they need attention), then pinned, then most
// recently updated. Rejected tombstones are hidden.
func (s *AgentMemoryStore) List(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status IN ('active','pending')
		 ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END, pinned DESC, updated_at DESC`,
		scope.String(), project)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory: %w", err)
	}
	defer rows.Close()
	var items []agentmemory.Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory: %w", err)
	}
	return items, nil
}

// Get returns one item by id.
func (s *AgentMemoryStore) Get(ctx context.Context, id string) (agentmemory.Item, bool, error) {
	item, err := scanItem(s.db.QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

// SetStatus moves an item through the review lifecycle (approve → active,
// reject → rejected). Rejecting drops the content to a bare tombstone that still
// blocks re-proposal by digest but carries no text.
func (s *AgentMemoryStore) SetStatus(ctx context.Context, id string, status agentmemory.Status, now time.Time) error {
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE agent_memory_items SET status = ?, updated_at = ? WHERE id = ?`,
		status.String(), now.UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("sqlite: set agent memory status: %w", err)
	}
	return affectedOne(result, "set status")
}

// SetPinned pins or unpins an item; pinned items are always injected and never
// auto-pruned.
func (s *AgentMemoryStore) SetPinned(ctx context.Context, id string, pinned bool, now time.Time) error {
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE agent_memory_items SET pinned = ?, updated_at = ? WHERE id = ?`,
		boolToInt(pinned), now.UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("sqlite: pin agent memory: %w", err)
	}
	return affectedOne(result, "pin")
}

// UpdateContent edits an item's content, recomputes its digest, and clears the
// now-stale embedding so a later fold re-embeds it.
func (s *AgentMemoryStore) UpdateContent(ctx context.Context, id, content string, now time.Time) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("sqlite: agent memory content is required")
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE agent_memory_items SET content = ?, digest = ?, embedding = x'', updated_at = ? WHERE id = ?`,
		content, digestOf(content), now.UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("sqlite: edit agent memory: %w", err)
	}
	return affectedOne(result, "edit")
}

// Delete removes an item outright.
func (s *AgentMemoryStore) Delete(ctx context.Context, id string) error {
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM agent_memory_items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete agent memory: %w", err)
	}
	return affectedOne(result, "delete")
}

// Add stores a user-authored active item. A digest collision with an existing
// item returns that item unchanged rather than creating a duplicate.
func (s *AgentMemoryStore) Add(ctx context.Context, scope agentmemory.Scope, project, content string, now time.Time) (agentmemory.Item, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return agentmemory.Item{}, errors.New("sqlite: agent memory content is required")
	}
	digest := digestOf(content)
	id, err := newMemoryID()
	if err != nil {
		return agentmemory.Item{}, err
	}
	nano := now.UTC().UnixNano()
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT OR IGNORE INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, pinned, session_id, day, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'user', 'active', 0, '', ?, ?, ?)`,
		id, scope.String(), project, content, digest, now.UTC().Format(time.DateOnly), nano, nano); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: add agent memory: %w", err)
	}
	item, _, err := s.itemByDigest(ctx, scope, project, digest)
	return item, err
}

func (s *AgentMemoryStore) itemByDigest(ctx context.Context, scope agentmemory.Scope, project, digest string) (agentmemory.Item, bool, error) {
	item, err := scanItem(conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE scope = ? AND project = ? AND digest = ?`,
		scope.String(), project, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

// affectedOne maps a zero-row update/delete to [agentmemory.ErrNotFound].
func affectedOne(result sql.Result, op string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect agent memory %s: %w", op, err)
	}
	if n == 0 {
		return agentmemory.ErrNotFound
	}
	return nil
}

// SetEmbeddings stores a content vector for each item id, ignoring ids that no
// longer exist (a concurrent reconcile may have pruned one).
func (s *AgentMemoryStore) SetEmbeddings(ctx context.Context, vectors map[string][]float32) error {
	if len(vectors) == 0 {
		return nil
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		for id, vec := range vectors {
			if _, err := conn(ctx, s.db).ExecContext(ctx,
				`UPDATE agent_memory_items SET embedding = ? WHERE id = ?`, encodeVec(vec), id); err != nil {
				return fmt.Errorf("sqlite: set agent memory embedding: %w", err)
			}
		}
		return nil
	})
}
