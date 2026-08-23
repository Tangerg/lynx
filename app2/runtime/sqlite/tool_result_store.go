package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
)

func insertToolResult(ctx context.Context, transaction *sql.Tx, record toolresult.Record) error {
	if err := record.Validate(); err != nil { return err }
	body, err := json.Marshal(record)
	if err != nil { return err }
	_, err = transaction.ExecContext(ctx, `INSERT INTO tool_results(id,item_id,session_id,body,created_at) VALUES(?,?,?,?,?)`, record.ID, record.ItemID, record.SessionID, string(body), encodeTime(record.CreatedAt))
	if err != nil { return fmt.Errorf("sqlite: insert tool result %s: %w", record.ID, err) }
	return nil
}

func (database *Database) ReadToolResult(ctx context.Context, sessionID, id string) (toolresult.Record, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM tool_results WHERE id=? AND session_id=?`, id, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) { return toolresult.Record{}, toolresult.ErrNotFound }
	if err != nil { return toolresult.Record{}, err }
	var record toolresult.Record
	if err := json.Unmarshal([]byte(body), &record); err != nil { return toolresult.Record{}, err }
	if err := record.Validate(); err != nil { return toolresult.Record{}, err }
	return record, nil
}
