package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

func newMemoryID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("sqlite: mint memory id: %w", err)
	}
	return "mem_" + hex.EncodeToString(buf[:]), nil
}

func encodeVec(vector []float32) []byte {
	encoded := make([]byte, 4*len(vector))
	for index, value := range vector {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value))
	}
	return encoded
}

func decodeVec(encoded []byte) ([]float32, error) {
	if len(encoded) == 0 || len(encoded)%4 != 0 {
		return nil, errors.New("sqlite: invalid agent memory embedding encoding")
	}
	vector := make([]float32, len(encoded)/4)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[index*4:]))
	}
	if err := agentmemory.ValidateEmbeddingVector(vector); err != nil {
		return nil, fmt.Errorf("sqlite: decode agent memory embedding: %w", err)
	}
	return vector, nil
}

// reconcileItems applies the domain fold ([agentmemory.Fold]) to the project's
// auto-origin items: prune the stale pending proposals it flags, insert the new
// curated facts as pending proposals. The review invariants (tombstone,
// active-sticky, pending-default, digest identity) live in the domain; this is
// the persistence that carries the plan out.
func (s *AgentMemoryStore) reconcileItems(ctx context.Context, project string, contents []string, now time.Time) error {
	existing, err := s.autoItems(ctx, project)
	if err != nil {
		return err
	}
	plan := agentmemory.Fold(existing, contents)
	for _, id := range plan.PruneIDs {
		if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM agent_memory_items WHERE id = ?`, id); err != nil {
			return fmt.Errorf("sqlite: prune agent memory item: %w", err)
		}
	}
	for _, content := range plan.InsertContents {
		id, err := newMemoryID()
		if err != nil {
			return err
		}
		item, err := agentmemory.NewProposal(id, project, content, now)
		if err != nil {
			return err
		}
		if _, err := s.insertItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// autoItems fetches the project's auto-origin, unpinned items (id + content +
// status) the fold reconciles over.
func (s *AgentMemoryStore) autoItems(ctx context.Context, project string) ([]agentmemory.Item, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT id, content, status FROM agent_memory_items
		 WHERE scope = 'project' AND project = ? AND origin = 'auto' AND pinned = 0`, project)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	defer rows.Close()
	var items []agentmemory.Item
	for rows.Next() {
		var (
			item       agentmemory.Item
			statusText string
		)
		if err := rows.Scan(&item.ID, &item.Content, &statusText); err != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", err)
		}
		item.Status, err = agentmemory.ParseStatus(statusText)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode agent memory item %q status: %w", item.ID, err)
		}
		content, err := agentmemory.NormalizeContent(item.Content)
		if err != nil || content != item.Content {
			if err == nil {
				err = errors.New("content is not canonical")
			}
			return nil, fmt.Errorf("sqlite: decode invalid agent memory item %q: %w", item.ID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
}

// insertItem writes a constructed item. OR IGNORE: a pinned or user item may
// already hold this content under the unique (scope, project, digest) index —
// keep it, don't duplicate.
func (s *AgentMemoryStore) insertItem(ctx context.Context, item agentmemory.Item) (bool, error) {
	if err := item.Validate(); err != nil {
		return false, fmt.Errorf("sqlite: insert invalid agent memory item: %w", err)
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT OR IGNORE INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, pinned, session_id, day, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Scope.String(), item.Project, item.Content, agentmemory.Digest(item.Content),
		item.Origin.String(), item.Status.String(), boolToInt(item.Pinned), item.SessionID, item.Day,
		item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano())
	if err != nil {
		return false, fmt.Errorf("sqlite: insert agent memory item: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect agent memory insert: %w", err)
	}
	return inserted == 1, nil
}

const agentMemoryItemColumns = `id, scope, project, content, origin, status, pinned, session_id, day, created_at, updated_at`

// scanItem decodes one item's base columns (embedding excluded — see
// [AgentMemoryStore.SearchCorpus] for the search path that reads it).
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
	return decodeItem(item, scopeText, originText, statusText, pinned, createdAt, updatedAt)
}

// decodeItem is the single persistence boundary for a fully loaded memory
// item. Both ordinary reads and search reads pass through the same closed
// vocabulary and domain-invariant checks.
func decodeItem(
	item agentmemory.Item,
	scopeText, originText, statusText string,
	pinned int,
	createdAt, updatedAt int64,
) (agentmemory.Item, error) {
	var err error
	item.Scope, err = agentmemory.ParseScope(scopeText)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item %q scope: %w", item.ID, err)
	}
	item.Origin, err = agentmemory.ParseOrigin(originText)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item %q origin: %w", item.ID, err)
	}
	item.Status, err = agentmemory.ParseStatus(statusText)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item %q status: %w", item.ID, err)
	}
	item.Pinned = pinned != 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if err := item.Validate(); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode invalid agent memory item %q: %w", item.ID, err)
	}
	return item, nil
}

// Items lists the active items for (scope, project): pinned first, then most
// recently updated. Pending and rejected items are excluded — only approved
// memory is injected into the prompt.
func (s *AgentMemoryStore) Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return nil, err
	}
	return s.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active'
		 ORDER BY pinned DESC, updated_at DESC`, "agent memory items", token, project)
}

// SearchCorpus lists the active exact-project and user-scoped items visible
// from one project context, with their embedding cache decoded. Fetching both
// partitions in one snapshot lets the application rank one combined corpus.
func (s *AgentMemoryStore) SearchCorpus(ctx context.Context, project string) ([]agentmemory.Item, error) {
	if _, err := memoryPartition(agentmemory.ScopeProject, project); err != nil {
		return nil, err
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+`, embedding_space, embedding
		 FROM agent_memory_items
		 WHERE status = 'active' AND (
		       (scope = 'project' AND project = ?) OR
		       (scope = 'user' AND project = '')
		 )`, project)
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
			space                             string
			blob                              []byte
		)
		if err := rows.Scan(&item.ID, &scopeText, &item.Project, &item.Content, &originText, &statusText,
			&pinned, &item.SessionID, &item.Day, &createdAt, &updatedAt, &space, &blob); err != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", err)
		}
		item, err = decodeItem(item, scopeText, originText, statusText, pinned, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		if space != "" || len(blob) != 0 {
			vector, decodeErr := decodeVec(blob)
			if decodeErr != nil {
				return nil, decodeErr
			}
			item.EmbeddingSpace = space
			item.Embedding = vector
			if validateErr := item.Validate(); validateErr != nil {
				return nil, fmt.Errorf("sqlite: decode invalid agent memory search item %q: %w", item.ID, validateErr)
			}
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
	token, err := memoryPartition(scope, project)
	if err != nil {
		return nil, err
	}
	return s.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status IN ('active','pending')
		 ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END, pinned DESC, updated_at DESC`,
		"agent memory", token, project)
}

func (s *AgentMemoryStore) listItems(ctx context.Context, query, operation string, args ...any) ([]agentmemory.Item, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list %s: %w", operation, err)
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
		return nil, fmt.Errorf("sqlite: iterate %s: %w", operation, err)
	}
	return items, nil
}

// Get returns one item by id.
func (s *AgentMemoryStore) Get(ctx context.Context, id string) (agentmemory.Item, bool, error) {
	item, err := scanItem(conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

// Update applies the review surface's content/pin patch atomically. Content
// edits clear stale embeddings; either validation or persistence failure rolls
// back every requested field, so callers never observe a half-applied update.
func (s *AgentMemoryStore) Update(ctx context.Context, id string, content *string, pinned *bool, now time.Time) (agentmemory.Item, error) {
	var updated agentmemory.Item
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		if content != nil {
			if err := s.UpdateContent(ctx, id, *content, now); err != nil {
				return err
			}
		}
		if pinned != nil {
			if err := s.SetPinned(ctx, id, *pinned, now); err != nil {
				return err
			}
		}
		item, found, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return agentmemory.ErrNotFound
		}
		updated = item
		return nil
	})
	if err != nil {
		return agentmemory.Item{}, err
	}
	return updated, nil
}

// Review atomically resolves one pending proposal. A user-authored, already
// reviewed, or rejected item cannot be rewritten through the review command.
func (s *AgentMemoryStore) Review(ctx context.Context, id string, decision agentmemory.ReviewDecision, now time.Time) error {
	status, err := decision.Result()
	if err != nil {
		return err
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		result, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE agent_memory_items SET status = ?, updated_at = ? WHERE id = ? AND status = 'pending'`,
			status.String(), now.UTC().UnixNano(), id)
		if err != nil {
			return fmt.Errorf("sqlite: review agent memory item: %w", err)
		}
		matched, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect agent memory review: %w", err)
		}
		if matched == 1 {
			return nil
		}
		var stored string
		if err := conn(ctx, s.db).QueryRowContext(ctx,
			`SELECT status FROM agent_memory_items WHERE id = ?`, id).Scan(&stored); errors.Is(err, sql.ErrNoRows) {
			return agentmemory.ErrNotFound
		} else if err != nil {
			return fmt.Errorf("sqlite: inspect agent memory review target: %w", err)
		}
		current, err := agentmemory.ParseStatus(stored)
		if err != nil {
			return fmt.Errorf("sqlite: decode agent memory review target %q: %w", id, err)
		}
		return fmt.Errorf("%w: item %q is %s", agentmemory.ErrNotPending, id, current)
	})
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
	content, err := agentmemory.NormalizeContent(content)
	if err != nil {
		return fmt.Errorf("sqlite: edit agent memory: %w", err)
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE agent_memory_items SET content = ?, digest = ?, embedding_space = '', embedding = x'', updated_at = ? WHERE id = ?`,
		content, agentmemory.Digest(content), now.UTC().UnixNano(), id)
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
func (s *AgentMemoryStore) Add(ctx context.Context, scope agentmemory.Scope, project, content string, now time.Time) (agentmemory.Item, bool, error) {
	if strings.TrimSpace(content) == "" {
		return agentmemory.Item{}, false, errors.New("sqlite: agent memory content is required")
	}
	id, err := newMemoryID()
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	item, err := agentmemory.NewUserItem(id, scope, project, content, now)
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	created, err := s.insertItem(ctx, item)
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	stored, _, err := s.itemByDigest(ctx, scope, project, agentmemory.Digest(item.Content))
	return stored, created, err
}

func (s *AgentMemoryStore) itemByDigest(ctx context.Context, scope agentmemory.Scope, project, digest string) (agentmemory.Item, bool, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	item, err := scanItem(conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE scope = ? AND project = ? AND digest = ?`,
		token, project, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

func memoryPartition(scope agentmemory.Scope, project string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	switch scope {
	case agentmemory.ScopeProject:
		if strings.TrimSpace(project) == "" {
			return "", errors.New("agentmemory: project scope requires a project")
		}
	case agentmemory.ScopeUser:
		if project != "" {
			return "", errors.New("agentmemory: user scope forbids a project")
		}
	}
	return scope.String(), nil
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

// SetEmbeddings caches vectors only while the exact item content remains
// active. A concurrent edit, review, or reconcile therefore makes the update a
// no-op instead of attaching a late vector to different content.
func (s *AgentMemoryStore) SetEmbeddings(ctx context.Context, updates []agentmemory.EmbeddingUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(updates))
	for _, update := range updates {
		if err := update.Validate(); err != nil {
			return fmt.Errorf("sqlite: invalid agent memory embedding update: %w", err)
		}
		if _, duplicate := seen[update.ItemID]; duplicate {
			return fmt.Errorf("sqlite: duplicate agent memory embedding update %q", update.ItemID)
		}
		seen[update.ItemID] = struct{}{}
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		for _, update := range updates {
			if _, err := conn(ctx, s.db).ExecContext(ctx,
				`UPDATE agent_memory_items SET embedding_space = ?, embedding = ?
				 WHERE id = ? AND digest = ? AND status = 'active'`,
				update.Space, encodeVec(update.Vector), update.ItemID, update.ContentDigest); err != nil {
				return fmt.Errorf("sqlite: set agent memory embedding: %w", err)
			}
		}
		return nil
	})
}
