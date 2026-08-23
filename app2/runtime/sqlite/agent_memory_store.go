package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/agentmemory"
)

const agentMemoryColumns = `
	id, scope, project_path, content, digest, origin, status, pinned,
	session_id, day, revision, created_at, updated_at`

type agentMemoryScanner interface {
	Scan(...any) error
}

func (database *Database) ListAgentMemory(
	ctx context.Context,
	scope agentmemory.Scope,
	project string,
) ([]agentmemory.Item, error) {
	rows, err := database.database.QueryContext(ctx, `
		SELECT `+agentMemoryColumns+`
		FROM agent_memory
		WHERE scope = ? AND project_path = ? AND status != 'rejected'
		ORDER BY
			CASE status WHEN 'pending' THEN 0 ELSE 1 END,
			pinned DESC,
			updated_at DESC,
			id
		LIMIT ?`,
		scope,
		project,
		agentmemory.MaxVisiblePerTarget+1,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list AgentMemory: %w", err)
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
		return nil, fmt.Errorf("sqlite: iterate AgentMemory: %w", err)
	}
	if len(values) > agentmemory.MaxVisiblePerTarget {
		return nil, errors.New("sqlite: AgentMemory target exceeds its complete-list bound")
	}
	return values, nil
}

func (database *Database) AddAgentMemory(
	ctx context.Context,
	item agentmemory.Item,
) (agentmemory.Item, bool, error) {
	if err := item.Validate(); err != nil {
		return agentmemory.Item{}, false, err
	}
	type result struct {
		item    agentmemory.Item
		changed bool
	}
	value, err := runAgentMemoryTx(ctx, database, func(tx *sql.Tx) (result, error) {
		existing, found, err := findAgentMemoryDigest(
			ctx, tx, item.Scope, item.Project, item.Digest,
		)
		if err != nil {
			return result{}, err
		}
		if found {
			// Automatic proposals are idempotent across every lifecycle state.
			// In particular, a rejected tombstone suppresses re-proposal of the
			// same fact. An explicit user addition may deliberately revive it.
			if item.Origin == agentmemory.OriginAuto ||
				existing.Status == agentmemory.StatusActive {
				return result{item: existing}, nil
			}
			if existing.Status == agentmemory.StatusRejected {
				visible, err := countVisibleAgentMemory(
					ctx, tx, item.Scope, item.Project,
				)
				if err != nil {
					return result{}, err
				}
				if visible >= agentmemory.MaxVisiblePerTarget {
					return result{}, agentmemory.ErrTargetFull
				}
			}
			existing.Content = item.Content
			existing.Digest = item.Digest
			existing.Origin = agentmemory.OriginUser
			existing.Status = agentmemory.StatusActive
			existing.Pinned = false
			existing.SessionID = ""
			existing.Day = item.Day
			existing.Revision++
			existing.UpdatedAt = item.UpdatedAt
			if err := existing.Validate(); err != nil {
				return result{}, err
			}
			if err := updateAgentMemory(ctx, tx, existing, existing.Revision-1); err != nil {
				return result{}, err
			}
			return result{item: existing, changed: true}, nil
		}
		visible, err := countVisibleAgentMemory(
			ctx, tx, item.Scope, item.Project,
		)
		if err != nil {
			return result{}, err
		}
		if visible >= agentmemory.MaxVisiblePerTarget {
			return result{}, agentmemory.ErrTargetFull
		}
		if err := insertAgentMemory(ctx, tx, item); err != nil {
			return result{}, err
		}
		return result{item: item, changed: true}, nil
	})
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return value.item, value.changed, nil
}

func countVisibleAgentMemory(
	ctx context.Context,
	tx *sql.Tx,
	scope agentmemory.Scope,
	project string,
) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_memory
		WHERE scope = ? AND project_path = ? AND status != 'rejected'`,
		scope,
		project,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("sqlite: count AgentMemory target: %w", err)
	}
	return count, nil
}

func (database *Database) ReviewAgentMemory(
	ctx context.Context,
	id string,
	decision agentmemory.ReviewDecision,
	now time.Time,
) (bool, error) {
	return runAgentMemoryTx(ctx, database, func(tx *sql.Tx) (bool, error) {
		item, err := getAgentMemory(ctx, tx, id)
		if err != nil {
			return false, err
		}
		updated, err := item.Review(decision, now)
		if err != nil {
			return false, err
		}
		if err := updateAgentMemory(ctx, tx, updated, item.Revision); err != nil {
			return false, err
		}
		if updated.Status == agentmemory.StatusRejected {
			if err := pruneRejectedAgentMemory(
				ctx,
				tx,
				updated.Scope,
				updated.Project,
				updated.ID,
			); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

func pruneRejectedAgentMemory(
	ctx context.Context,
	tx *sql.Tx,
	scope agentmemory.Scope,
	project string,
	preservedID string,
) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM agent_memory
		WHERE id IN (
			SELECT id
			FROM agent_memory
			WHERE scope = ? AND project_path = ? AND status = 'rejected'
				AND id != ?
			ORDER BY updated_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`,
		scope,
		project,
		preservedID,
		agentmemory.MaxRejectedPerTarget-1,
	); err != nil {
		return fmt.Errorf("sqlite: prune AgentMemory tombstones: %w", err)
	}
	return nil
}

func (database *Database) UpdateAgentMemory(
	ctx context.Context,
	id string,
	patch agentmemory.Patch,
	now time.Time,
) (agentmemory.Item, bool, error) {
	type result struct {
		item    agentmemory.Item
		changed bool
	}
	value, err := runAgentMemoryTx(ctx, database, func(tx *sql.Tx) (result, error) {
		item, err := getAgentMemory(ctx, tx, id)
		if err != nil {
			return result{}, err
		}
		updated, changed, err := item.Apply(patch, now)
		if err != nil {
			return result{}, err
		}
		if !changed {
			return result{item: item}, nil
		}
		duplicate, found, err := findAgentMemoryDigest(
			ctx, tx, updated.Scope, updated.Project, updated.Digest,
		)
		if err != nil {
			return result{}, err
		}
		if found && duplicate.ID != updated.ID {
			return result{}, agentmemory.ErrDuplicate
		}
		if err := updateAgentMemory(ctx, tx, updated, item.Revision); err != nil {
			return result{}, err
		}
		return result{item: updated, changed: true}, nil
	})
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return value.item, value.changed, nil
}

func (database *Database) DeleteAgentMemory(
	ctx context.Context,
	id string,
) (bool, error) {
	return runAgentMemoryTx(ctx, database, func(tx *sql.Tx) (bool, error) {
		item, err := getAgentMemory(ctx, tx, id)
		if err != nil {
			return false, err
		}
		if item.Status != agentmemory.StatusActive {
			return false, fmt.Errorf(
				"%w: only active memory may be deleted",
				agentmemory.ErrInvalidMutation,
			)
		}
		result, err := tx.ExecContext(
			ctx,
			`DELETE FROM agent_memory WHERE id = ? AND revision = ?`,
			id,
			item.Revision,
		)
		if err != nil {
			return false, fmt.Errorf("sqlite: delete AgentMemory: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("sqlite: inspect AgentMemory delete: %w", err)
		}
		if changed != 1 {
			return false, errors.New(
				"sqlite: AgentMemory revision changed inside transaction",
			)
		}
		return true, nil
	})
}

func getAgentMemory(
	ctx context.Context,
	tx *sql.Tx,
	id string,
) (agentmemory.Item, error) {
	item, err := scanAgentMemory(tx.QueryRowContext(ctx, `
		SELECT `+agentMemoryColumns+` FROM agent_memory WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, agentmemory.ErrNotFound
	}
	return item, err
}

func findAgentMemoryDigest(
	ctx context.Context,
	tx *sql.Tx,
	scope agentmemory.Scope,
	project string,
	digest string,
) (agentmemory.Item, bool, error) {
	item, err := scanAgentMemory(tx.QueryRowContext(ctx, `
		SELECT `+agentMemoryColumns+`
		FROM agent_memory
		WHERE scope = ? AND project_path = ? AND digest = ?`,
		scope,
		project,
		digest,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	return item, err == nil, err
}

func updateAgentMemory(
	ctx context.Context,
	tx *sql.Tx,
	item agentmemory.Item,
	expectedRevision uint64,
) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE agent_memory SET
			content = ?, digest = ?, origin = ?, status = ?, pinned = ?,
			session_id = ?, day = ?, revision = ?, updated_at = ?
		WHERE id = ? AND revision = ?`,
		item.Content,
		item.Digest,
		item.Origin,
		item.Status,
		item.Pinned,
		item.SessionID,
		item.Day,
		item.Revision,
		encodeTime(item.UpdatedAt),
		item.ID,
		expectedRevision,
	)
	if err != nil {
		return fmt.Errorf("sqlite: update AgentMemory: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect AgentMemory update: %w", err)
	}
	if changed != 1 {
		return errors.New("sqlite: AgentMemory revision changed inside transaction")
	}
	return nil
}

func insertAgentMemory(
	ctx context.Context,
	tx *sql.Tx,
	item agentmemory.Item,
) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO agent_memory (
			id, scope, project_path, content, digest, origin, status, pinned,
			session_id, day, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID,
		item.Scope,
		item.Project,
		item.Content,
		item.Digest,
		item.Origin,
		item.Status,
		item.Pinned,
		item.SessionID,
		item.Day,
		item.Revision,
		encodeTime(item.CreatedAt),
		encodeTime(item.UpdatedAt),
	); err != nil {
		return fmt.Errorf("sqlite: insert AgentMemory: %w", err)
	}
	return nil
}

func scanAgentMemory(scanner agentMemoryScanner) (agentmemory.Item, error) {
	var item agentmemory.Item
	var scope string
	var origin string
	var status string
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&item.ID,
		&scope,
		&item.Project,
		&item.Content,
		&item.Digest,
		&origin,
		&status,
		&item.Pinned,
		&item.SessionID,
		&item.Day,
		&item.Revision,
		&createdAt,
		&updatedAt,
	); err != nil {
		return agentmemory.Item{}, err
	}
	item.Scope = agentmemory.Scope(scope)
	item.Origin = agentmemory.Origin(origin)
	item.Status = agentmemory.Status(status)
	var err error
	item.CreatedAt, err = decodeTime(createdAt)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode AgentMemory creation time: %w", err)
	}
	item.UpdatedAt, err = decodeTime(updatedAt)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode AgentMemory update time: %w", err)
	}
	if err := item.Validate(); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: invalid AgentMemory item %q: %w", item.ID, err)
	}
	return item, nil
}

func runAgentMemoryTx[T any](
	ctx context.Context,
	database *Database,
	operation func(*sql.Tx) (T, error),
) (T, error) {
	var zero T
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return zero, fmt.Errorf("sqlite: begin AgentMemory transaction: %w", err)
	}
	defer transaction.Rollback()
	value, err := operation(transaction)
	if err != nil {
		return zero, err
	}
	if err := transaction.Commit(); err != nil {
		return zero, fmt.Errorf("sqlite: commit AgentMemory transaction: %w", err)
	}
	return value, nil
}
