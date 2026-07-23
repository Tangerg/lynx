package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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
		if err := s.insertItem(ctx, agentmemory.NewProposal(id, project, content, now)); err != nil {
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
		item.Status = agentmemory.ParseStatus(statusText)
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
func (s *AgentMemoryStore) insertItem(ctx context.Context, item agentmemory.Item) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT OR IGNORE INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, pinned, session_id, day, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Scope.String(), item.Project, item.Content, agentmemory.Digest(item.Content),
		item.Origin.String(), item.Status.String(), boolToInt(item.Pinned), item.SessionID, item.Day,
		item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano()); err != nil {
		return fmt.Errorf("sqlite: insert agent memory item: %w", err)
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
	return s.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active'
		 ORDER BY pinned DESC, updated_at DESC`, "agent memory items", scope.String(), project)
}

// ItemsForSearch lists the active (scope, project) items with their embedding
// decoded, for in-process keyword + vector ranking. Only approved memory is
// searchable.
func (s *AgentMemoryStore) ItemsForSearch(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
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
	return s.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active' AND length(embedding) = 0`, "unembedded agent memory items", scope.String(), project)
}

// List returns the (scope, project) items the review surface shows: active and
// pending, ordered pending-first (they need attention), then pinned, then most
// recently updated. Rejected tombstones are hidden.
func (s *AgentMemoryStore) List(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	return s.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status IN ('active','pending')
		 ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END, pinned DESC, updated_at DESC`,
		"agent memory", scope.String(), project)
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
func (s *AgentMemoryStore) Add(ctx context.Context, scope agentmemory.Scope, project, content string, now time.Time) (agentmemory.Item, error) {
	if strings.TrimSpace(content) == "" {
		return agentmemory.Item{}, errors.New("sqlite: agent memory content is required")
	}
	id, err := newMemoryID()
	if err != nil {
		return agentmemory.Item{}, err
	}
	item := agentmemory.NewUserItem(id, scope, project, content, now)
	if err := s.insertItem(ctx, item); err != nil {
		return agentmemory.Item{}, err
	}
	stored, _, err := s.itemByDigest(ctx, scope, project, agentmemory.Digest(item.Content))
	return stored, err
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
