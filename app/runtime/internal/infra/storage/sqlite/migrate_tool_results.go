package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type schemaV6HistoryItem struct {
	sessionID string
	itemID    string
	payload   string
}

func migrateToolResultBindings(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: begin tool-result binding migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS history_items (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  TEXT    NOT NULL,
			run_id      TEXT    NOT NULL DEFAULT '',
			item_id     TEXT    NOT NULL UNIQUE,
			created_at  INTEGER NOT NULL,
			payload     TEXT    NOT NULL,
			offload_id  TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS tool_result_blobs (
		id          TEXT    PRIMARY KEY,
		session_id  TEXT    NOT NULL DEFAULT '',
		item_id     TEXT    NOT NULL DEFAULT '',
		tool_name   TEXT    NOT NULL DEFAULT '',
		preview     TEXT    NOT NULL DEFAULT '',
		body        TEXT    NOT NULL,
		created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("sqlite: prepare tool-result binding schema: %w", err)
		}
	}
	for _, column := range []struct {
		table     string
		name      string
		statement string
	}{
		{table: "history_items", name: "offload_id", statement: `ALTER TABLE history_items ADD COLUMN offload_id TEXT NOT NULL DEFAULT ''`},
		{table: "tool_result_blobs", name: "item_id", statement: `ALTER TABLE tool_result_blobs ADD COLUMN item_id TEXT NOT NULL DEFAULT ''`},
		{table: "tool_result_blobs", name: "preview", statement: `ALTER TABLE tool_result_blobs ADD COLUMN preview TEXT NOT NULL DEFAULT ''`},
	} {
		exists, err := tableHasColumn(tx, column.table, column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(column.statement); err != nil {
			return fmt.Errorf("sqlite: add %s.%s: %w", column.table, column.name, err)
		}
	}
	if err := backfillToolResultBindings(tx); err != nil {
		return err
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_tool_result_blobs_session ON tool_result_blobs(session_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_result_blobs_item ON tool_result_blobs(item_id) WHERE item_id != ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_history_items_offload ON history_items(offload_id) WHERE offload_id != ''`,
		fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion),
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("sqlite: finalize tool-result binding migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit tool-result binding migration: %w", err)
	}
	return nil
}

func tableHasColumn(tx *sql.Tx, table, column string) (bool, error) {
	var query string
	switch table {
	case "history_items":
		query = `PRAGMA table_info(history_items)`
	case "tool_result_blobs":
		query = `PRAGMA table_info(tool_result_blobs)`
	default:
		return false, fmt.Errorf("sqlite: inspect unsupported table %q", table)
	}
	rows, err := tx.Query(query)
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("sqlite: scan %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("sqlite: inspect %s columns: %w", table, err)
	}
	return false, nil
}

func backfillToolResultBindings(tx *sql.Tx) error {
	bySession, err := schemaV6BlobIDs(tx)
	if err != nil {
		return err
	}
	items, err := schemaV6HistoryItems(tx)
	if err != nil {
		return err
	}
	boundIDs := make(map[offload.ID]string)
	for _, stored := range items {
		var item transcript.Item
		if err := json.Unmarshal([]byte(stored.payload), &item); err != nil {
			return fmt.Errorf("sqlite: decode history item %q during tool-result migration: %w", stored.itemID, err)
		}
		if item.Tool == nil {
			continue
		}
		preview, ok := item.Tool.Result.(string)
		if !ok {
			continue
		}
		var matched offload.ID
		for _, id := range bySession[stored.sessionID] {
			if !schemaV6PreviewReferences(preview, id) {
				continue
			}
			if matched != "" {
				return fmt.Errorf("sqlite: history item %q references multiple offloaded results", stored.itemID)
			}
			matched = id
		}
		if matched == "" {
			continue
		}
		if owner, duplicate := boundIDs[matched]; duplicate {
			return fmt.Errorf("sqlite: schema-v6 tool result %q is referenced by both history items %q and %q", matched, owner, stored.itemID)
		}
		boundIDs[matched] = stored.itemID
		itemResult, err := tx.Exec(`UPDATE history_items SET offload_id = ? WHERE item_id = ?`, matched, stored.itemID)
		if err != nil {
			return fmt.Errorf("sqlite: bind migrated history item %q: %w", stored.itemID, err)
		}
		if err := requireOneMigratedRow(itemResult, "history item", stored.itemID); err != nil {
			return err
		}
		blobResult, err := tx.Exec(
			`UPDATE tool_result_blobs SET item_id = ?, preview = ? WHERE id = ? AND session_id = ?`,
			stored.itemID, preview, matched, stored.sessionID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: bind migrated tool result %q: %w", matched, err)
		}
		if err := requireOneMigratedRow(blobResult, "tool result", matched.String()); err != nil {
			return err
		}
	}
	return nil
}

func requireOneMigratedRow(result sql.Result, kind, id string) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect migrated %s %q: %w", kind, id, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: migrated %s %q updated %d rows, want 1", kind, id, changed)
	}
	return nil
}

func schemaV6BlobIDs(tx *sql.Tx) (map[string][]offload.ID, error) {
	rows, err := tx.Query(`SELECT session_id, id FROM tool_result_blobs`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list schema-v6 tool results: %w", err)
	}
	defer rows.Close()
	bySession := make(map[string][]offload.ID)
	for rows.Next() {
		var sessionID, rawID string
		if err := rows.Scan(&sessionID, &rawID); err != nil {
			return nil, fmt.Errorf("sqlite: scan schema-v6 tool result: %w", err)
		}
		id, err := offload.ParseID(rawID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: schema-v6 tool result ID %q: %w", rawID, err)
		}
		bySession[sessionID] = append(bySession[sessionID], id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list schema-v6 tool results: %w", err)
	}
	return bySession, nil
}

func schemaV6HistoryItems(tx *sql.Tx) ([]schemaV6HistoryItem, error) {
	rows, err := tx.Query(`SELECT session_id, item_id, payload FROM history_items`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list schema-v6 history items: %w", err)
	}
	defer rows.Close()
	var items []schemaV6HistoryItem
	for rows.Next() {
		var item schemaV6HistoryItem
		if err := rows.Scan(&item.sessionID, &item.itemID, &item.payload); err != nil {
			return nil, fmt.Errorf("sqlite: scan schema-v6 history item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list schema-v6 history items: %w", err)
	}
	return items, nil
}

func schemaV6PreviewReferences(preview string, id offload.ID) bool {
	marker := strings.Index(preview, " bytes offloaded to keep the context small")
	if marker < 0 {
		return false
	}
	reference := strings.Index(preview[marker:], `{"id":"`+id.String()+`"}`)
	if reference < 0 {
		return false
	}
	if newline := strings.IndexByte(preview[marker:], '\n'); newline >= 0 && reference > newline {
		return false
	}
	return true
}
