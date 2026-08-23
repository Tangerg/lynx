package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/conversation"
)

func insertConversationMessage(ctx context.Context, transaction *sql.Tx, record conversation.Record) error {
	if record.SessionID == "" || record.RunID == "" || record.Ordinal < 0 || len(record.Body) == 0 {
		return fmt.Errorf("sqlite: incomplete conversation message")
	}
	_, err := transaction.ExecContext(ctx, `INSERT INTO conversation_messages(session_id,run_id,ordinal,body) VALUES(?,?,?,?)`, record.SessionID, record.RunID, record.Ordinal, string(record.Body))
	if err != nil { return fmt.Errorf("sqlite: append conversation message %d: %w", record.Ordinal, err) }
	return nil
}

func (database *Database) ListConversationMessages(ctx context.Context, sessionID string) ([]conversation.Record, error) {
	through := -1
	var compacted conversation.Record
	err := database.database.QueryRowContext(ctx, `SELECT session_id,run_id,through_ordinal,summary_body FROM conversation_compactions WHERE session_id=? ORDER BY through_ordinal DESC LIMIT 1`, sessionID).Scan(&compacted.SessionID,&compacted.RunID,&through,&compacted.Body)
	if err != nil && !errors.Is(err, sql.ErrNoRows) { return nil, err }
	rows, err := database.database.QueryContext(ctx, `SELECT session_id,run_id,ordinal,body FROM conversation_messages WHERE session_id=? AND ordinal>? ORDER BY ordinal`, sessionID, through)
	if err != nil { return nil, err }
	defer rows.Close()
	values := make([]conversation.Record, 0)
	if through >= 0 { compacted.Ordinal = through; values = append(values, compacted) }
	for rows.Next() {
		var value conversation.Record
		if err := rows.Scan(&value.SessionID, &value.RunID, &value.Ordinal, &value.Body); err != nil { return nil, err }
		values = append(values, value)
	}
	return values, rows.Err()
}
