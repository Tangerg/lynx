package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/codebase"
)

func (database *Database) GetCodebaseIndex(
	ctx context.Context,
	workspace string,
) (codebase.Index, error) {
	var value codebase.Index
	var state string
	var indexed sql.NullString
	err := database.database.QueryRowContext(
		ctx,
		`SELECT state, COALESCE(operation_id, ''), COALESCE(model_id, ''),
		        file_count, chunk_count, truncated, indexed_at
		   FROM codebase_indexes
		  WHERE workspace_path = ?`,
		workspace,
	).Scan(
		&state,
		&value.OperationID,
		&value.ModelID,
		&value.FileCount,
		&value.ChunkCount,
		&value.Truncated,
		&indexed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return codebase.Index{Workspace: workspace, State: codebase.StateNone}, nil
	}
	if err != nil {
		return codebase.Index{}, fmt.Errorf("sqlite: get codebase index: %w", err)
	}
	value.Workspace = workspace
	value.State = codebase.State(state)
	if indexed.Valid {
		value.IndexedAt, err = decodeCodebaseTime(indexed.String)
		if err != nil {
			return codebase.Index{}, fmt.Errorf("sqlite: decode codebase indexed time: %w", err)
		}
	}
	return value, nil
}

func (database *Database) BeginCodebaseIndex(
	ctx context.Context,
	value codebase.Index,
) error {
	_, err := database.database.ExecContext(
		ctx,
		`INSERT INTO codebase_indexes(
		     workspace_path, state, operation_id, model_id, file_count,
		     chunk_count, truncated, indexed_at, updated_at
		 ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_path) DO UPDATE SET
		     state = excluded.state,
		     operation_id = excluded.operation_id,
		     model_id = excluded.model_id,
		     file_count = excluded.file_count,
		     chunk_count = excluded.chunk_count,
		     truncated = excluded.truncated,
		     indexed_at = excluded.indexed_at,
		     updated_at = excluded.updated_at`,
		value.Workspace,
		value.State,
		value.OperationID,
		value.ModelID,
		value.FileCount,
		value.ChunkCount,
		value.Truncated,
		encodeCodebaseTime(value.IndexedAt),
		encodeTime(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: begin codebase index: %w", err)
	}
	return nil
}

func (database *Database) CompleteCodebaseIndex(
	ctx context.Context,
	expectedOperation string,
	index codebase.Index,
	documents []codebase.Document,
) (bool, error) {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("sqlite: begin codebase completion: %w", err)
	}
	defer transaction.Rollback()

	applied, err := settleCodebaseIndex(ctx, transaction, expectedOperation, index)
	if err != nil || !applied {
		return applied, err
	}
	if _, err := transaction.ExecContext(
		ctx,
		`DELETE FROM codebase_documents WHERE workspace_path = ?`,
		index.Workspace,
	); err != nil {
		return false, fmt.Errorf("sqlite: clear codebase documents: %w", err)
	}
	now := encodeTime(time.Now())
	for ordinal, document := range documents {
		body, err := json.Marshal(document)
		if err != nil {
			return false, fmt.Errorf("sqlite: encode codebase document: %w", err)
		}
		key := fmt.Sprintf("%s#%08d", document.Path, ordinal)
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO codebase_documents(workspace_path, path, body, indexed_at)
			 VALUES(?, ?, ?, ?)`,
			index.Workspace,
			key,
			string(body),
			now,
		); err != nil {
			return false, fmt.Errorf("sqlite: insert codebase document: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("sqlite: commit codebase completion: %w", err)
	}
	return true, nil
}

func (database *Database) FailCodebaseIndex(
	ctx context.Context,
	expectedOperation string,
	index codebase.Index,
) (bool, error) {
	result, err := database.database.ExecContext(
		ctx,
		`UPDATE codebase_indexes
		    SET state = ?, operation_id = NULL, model_id = ?, file_count = ?,
		        chunk_count = ?, truncated = ?, indexed_at = ?, updated_at = ?
		  WHERE workspace_path = ? AND state = ? AND operation_id = ?`,
		index.State,
		index.ModelID,
		index.FileCount,
		index.ChunkCount,
		index.Truncated,
		encodeCodebaseTime(index.IndexedAt),
		encodeTime(time.Now()),
		index.Workspace,
		codebase.StateIndexing,
		expectedOperation,
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: fail codebase index: %w", err)
	}
	return exactlyOneRow(result)
}

func settleCodebaseIndex(
	ctx context.Context,
	transaction *sql.Tx,
	expectedOperation string,
	index codebase.Index,
) (bool, error) {
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE codebase_indexes
		    SET state = ?, operation_id = NULL, model_id = ?, file_count = ?,
		        chunk_count = ?, truncated = ?, indexed_at = ?, updated_at = ?
		  WHERE workspace_path = ? AND state = ? AND operation_id = ?`,
		index.State,
		index.ModelID,
		index.FileCount,
		index.ChunkCount,
		index.Truncated,
		encodeCodebaseTime(index.IndexedAt),
		encodeTime(time.Now()),
		index.Workspace,
		codebase.StateIndexing,
		expectedOperation,
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: settle codebase index: %w", err)
	}
	return exactlyOneRow(result)
}

func exactlyOneRow(result sql.Result) (bool, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect affected codebase rows: %w", err)
	}
	return count == 1, nil
}

func (database *Database) ListCodebaseDocuments(
	ctx context.Context,
	workspace string,
) ([]codebase.Document, error) {
	rows, err := database.database.QueryContext(
		ctx,
		`SELECT body FROM codebase_documents
		  WHERE workspace_path = ?
		  ORDER BY path`,
		workspace,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list codebase documents: %w", err)
	}
	values := make([]codebase.Document, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("sqlite: scan codebase document: %w", err)
		}
		var value codebase.Document
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("sqlite: decode codebase document: %w", err)
		}
		values = append(values, value)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("sqlite: finish codebase documents: %w", err)
	}
	return values, nil
}

func encodeCodebaseTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return encodeTime(*value)
}

func decodeCodebaseTime(value string) (*time.Time, error) {
	parsed, err := decodeTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
