package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
)

// ToolResultStore is the single full-body source for oversized tool outputs.
// Stage creates an unbound row before the tool returns; the run's atomic event
// commit binds it to the canonical transcript item and its inline preview.
type ToolResultStore struct {
	db *sql.DB
}

// NewToolResultStore wires a database with the current [Open]-installed schema
// to the offloaded-tool-result surface.
func NewToolResultStore(db *sql.DB) *ToolResultStore {
	return &ToolResultStore{db: db}
}

// Stage persists a body under its precomputed identity after the observer has
// verified that replacing it with a preview reduces model context. ToolName is
// retained for relationship validation and diagnostics.
func (t *ToolResultStore) Stage(ctx context.Context, stage toolresult.Stage) error {
	if err := stage.Validate(); err != nil {
		return fmt.Errorf("sqlite: stage tool result: %w", err)
	}
	_, err := conn(ctx, t.db).ExecContext(ctx,
		`INSERT INTO tool_result_blobs(id, session_id, tool_name, body, created_at)
		 VALUES (?, ?, ?, ?, strftime('%s','now'))`,
		stage.ID, stage.SessionID, stage.ToolName, stage.Body)
	if err != nil {
		return fmt.Errorf("sqlite: stage tool result %q: %w", stage.ID, err)
	}
	return nil
}

// Fetch returns the full offloaded body for (sessionID, id). found is false —
// with a nil error — when no such row exists (an unknown id is a recoverable
// miss the caller surfaces to the model, not a failure). Scoping the read by
// session id keeps one session from reading another's offloaded output.
func (t *ToolResultStore) Fetch(ctx context.Context, sessionID string, id toolresult.ID) (body string, found bool, err error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", false, errors.New("sqlite: fetch tool result requires a session ID")
	}
	if validateErr := id.Validate(); validateErr != nil {
		return "", false, fmt.Errorf("sqlite: fetch tool result: %w", validateErr)
	}
	err = conn(ctx, t.db).QueryRowContext(ctx,
		`SELECT body FROM tool_result_blobs WHERE id = ? AND session_id = ?`,
		id, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("sqlite: fetch tool result: %w", err)
	}
	return body, true, nil
}

// Bind attaches a freshly offloaded body to the transcript item committed in
// the same transaction. Exact retries are idempotent; another item or preview
// attempting to claim the ID is an identity conflict.
func (t *ToolResultStore) Bind(ctx context.Context, sessionID, itemID, preview string, ref toolresult.Ref) error {
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("sqlite: bind tool result: %w", err)
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(itemID) == "" || preview == "" {
		return errors.New("sqlite: bind tool result requires session, item, and preview")
	}
	result, err := conn(ctx, t.db).ExecContext(ctx,
		`UPDATE tool_result_blobs
		 SET item_id = ?, preview = ?
		 WHERE id = ? AND session_id = ?
		   AND (item_id = '' OR (item_id = ? AND preview = ?))`,
		itemID, preview, ref.ID, sessionID, itemID, preview,
	)
	if err != nil {
		return fmt.Errorf("sqlite: bind tool result %q: %w", ref.ID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect tool-result binding %q: %w", ref.ID, err)
	}
	if changed == 1 {
		return nil
	}
	var ownerSession, ownerItem, ownerPreview string
	err = conn(ctx, t.db).QueryRowContext(ctx,
		`SELECT session_id, item_id, preview FROM tool_result_blobs WHERE id = ?`, ref.ID,
	).Scan(&ownerSession, &ownerItem, &ownerPreview)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("sqlite: bind tool result %q: blob does not exist", ref.ID)
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect conflicting tool-result binding %q: %w", ref.ID, err)
	}
	return fmt.Errorf("%w: tool result %q belongs to session %q item %q with preview length %d",
		toolresult.ErrIdentityConflict, ref.ID, ownerSession, ownerItem, len(ownerPreview))
}

// List returns every transcript-bound blob owned by sessionID in stable order.
func (t *ToolResultStore) List(ctx context.Context, sessionID string) ([]toolresult.Blob, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("sqlite: list tool results requires a session ID")
	}
	rows, err := conn(ctx, t.db).QueryContext(ctx,
		`SELECT id, session_id, item_id, tool_name, preview, body, created_at
		 FROM tool_result_blobs
		 WHERE session_id = ? AND item_id != ''
		 ORDER BY created_at, id`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list tool results: %w", err)
	}
	defer rows.Close()
	var blobs []toolresult.Blob
	for rows.Next() {
		var blob toolresult.Blob
		var rawID string
		var createdAt int64
		if scanErr := rows.Scan(&rawID, &blob.SessionID, &blob.ItemID, &blob.ToolName, &blob.Preview, &blob.Body, &createdAt); scanErr != nil {
			return nil, fmt.Errorf("sqlite: scan tool result: %w", scanErr)
		}
		blob.ID, err = toolresult.ParseID(rawID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode tool-result ID %q: %w", rawID, err)
		}
		blob.CreatedAt = time.Unix(createdAt, 0).UTC()
		if err := blob.Validate(); err != nil {
			return nil, fmt.Errorf("sqlite: invalid stored tool result %q: %w", rawID, err)
		}
		blobs = append(blobs, blob)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list tool results: %w", err)
	}
	return blobs, nil
}

// Restore inserts one artifact blob under its exact identity. It never adopts
// an ID owned by another session.
func (t *ToolResultStore) Restore(ctx context.Context, blob toolresult.Blob) error {
	if err := blob.Validate(); err != nil {
		return fmt.Errorf("sqlite: restore tool result: %w", err)
	}
	_, err := conn(ctx, t.db).ExecContext(ctx,
		`INSERT INTO tool_result_blobs(id, session_id, item_id, tool_name, preview, body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		blob.ID, blob.SessionID, blob.ItemID, blob.ToolName, blob.Preview, blob.Body, blob.CreatedAt.Unix(),
	)
	if err == nil {
		return nil
	}
	var owner string
	ownerErr := conn(ctx, t.db).QueryRowContext(ctx,
		`SELECT session_id FROM tool_result_blobs WHERE id = ?`, blob.ID,
	).Scan(&owner)
	if ownerErr == nil {
		return fmt.Errorf("%w: tool result %q is already owned by session %q", toolresult.ErrIdentityConflict, blob.ID, owner)
	}
	if !errors.Is(ownerErr, sql.ErrNoRows) {
		return fmt.Errorf("sqlite: inspect tool-result restore conflict %q: %w", blob.ID, errors.Join(err, ownerErr))
	}
	return fmt.Errorf("sqlite: restore tool result %q: %w", blob.ID, err)
}

// Discard removes a staged blob only while it is still unbound. It is the
// compensation path for a failed atomic event commit: a concurrently or
// ambiguously committed binding is never deleted.
func (t *ToolResultStore) Discard(ctx context.Context, sessionID string, ref toolresult.Ref) error {
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("sqlite: discard tool result requires a session ID")
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("sqlite: discard tool result: %w", err)
	}
	if _, err := conn(ctx, t.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE id = ? AND session_id = ? AND item_id = ''`,
		ref.ID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: discard staged tool result %q: %w", ref.ID, err)
	}
	return nil
}

// PurgeUnbound removes staged blobs left by a process crash before their
// transcript event committed. It is safe only during startup, before tool
// execution begins; bound blobs are never touched.
func (t *ToolResultStore) PurgeUnbound(ctx context.Context) (int64, error) {
	result, err := conn(ctx, t.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE item_id = ''`,
	)
	if err != nil {
		return 0, fmt.Errorf("sqlite: purge staged tool results: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: inspect staged tool-result purge: %w", err)
	}
	return removed, nil
}

// DropSession removes every offloaded body owned by sessionID — the blob half
// of the session-delete cascade. It joins an ambient lifecycle write-set
// transaction through conn(ctx).
func (t *ToolResultStore) DropSession(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("sqlite: drop tool results requires a session ID")
	}
	if _, err := conn(ctx, t.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: drop session tool results: %w", err)
	}
	return nil
}
