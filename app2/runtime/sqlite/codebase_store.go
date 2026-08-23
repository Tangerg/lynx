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

func (database *Database) GetCodebaseIndex(ctx context.Context, workspace string) (codebase.Index, error) {
	var value codebase.Index
	var indexed sql.NullString
	err := database.database.QueryRowContext(ctx, `SELECT state,COALESCE(operation_id,''),COALESCE(model_id,''),file_count,chunk_count,truncated,indexed_at FROM codebase_indexes WHERE workspace_path=?`, workspace).Scan(&value.State, &value.OperationID, &value.ModelID, &value.FileCount, &value.ChunkCount, &value.Truncated, &indexed)
	if errors.Is(err, sql.ErrNoRows) { return codebase.Index{Workspace: workspace, State: "none"}, nil }
	if err != nil { return codebase.Index{}, err }
	value.Workspace = workspace
	if indexed.Valid { parsed, err := decodeTime(indexed.String); if err != nil { return codebase.Index{}, err }; value.IndexedAt = &parsed }
	return value, nil
}

func (database *Database) PutCodebaseIndexState(ctx context.Context, value codebase.Index) error {
	var indexed any
	if value.IndexedAt != nil { indexed = encodeTime(*value.IndexedAt) }
	_, err := database.database.ExecContext(ctx, `INSERT INTO codebase_indexes(workspace_path,state,operation_id,model_id,file_count,chunk_count,truncated,indexed_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(workspace_path) DO UPDATE SET state=excluded.state,operation_id=excluded.operation_id,model_id=excluded.model_id,file_count=excluded.file_count,chunk_count=excluded.chunk_count,truncated=excluded.truncated,indexed_at=excluded.indexed_at,updated_at=excluded.updated_at`, value.Workspace, value.State, value.OperationID, value.ModelID, value.FileCount, value.ChunkCount, value.Truncated, indexed, encodeTime(time.Now()))
	return err
}

func (database *Database) ReplaceCodebaseDocuments(ctx context.Context, index codebase.Index, documents []codebase.Document) error {
	transaction, err := database.database.BeginTx(ctx, nil); if err != nil { return err }; defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `DELETE FROM codebase_documents WHERE workspace_path=?`, index.Workspace); err != nil { return err }
	for ordinal, document := range documents {
		body, err := json.Marshal(document); if err != nil { return err }
		key := fmt.Sprintf("%s#%08d", document.Path, ordinal)
		if _, err := transaction.ExecContext(ctx, `INSERT INTO codebase_documents(workspace_path,path,body,indexed_at) VALUES(?,?,?,?)`, index.Workspace, key, string(body), encodeTime(time.Now())); err != nil { return err }
	}
	var indexed any; if index.IndexedAt != nil { indexed = encodeTime(*index.IndexedAt) }
	if _, err := transaction.ExecContext(ctx, `INSERT INTO codebase_indexes(workspace_path,state,operation_id,model_id,file_count,chunk_count,truncated,indexed_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(workspace_path) DO UPDATE SET state=excluded.state,operation_id=excluded.operation_id,model_id=excluded.model_id,file_count=excluded.file_count,chunk_count=excluded.chunk_count,truncated=excluded.truncated,indexed_at=excluded.indexed_at,updated_at=excluded.updated_at`, index.Workspace,index.State,index.OperationID,index.ModelID,index.FileCount,index.ChunkCount,index.Truncated,indexed,encodeTime(time.Now())); err != nil { return err }
	return transaction.Commit()
}

func (database *Database) ListCodebaseDocuments(ctx context.Context, workspace string) ([]codebase.Document, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body FROM codebase_documents WHERE workspace_path=? ORDER BY path`, workspace); if err != nil { return nil, err }; defer rows.Close()
	values := make([]codebase.Document, 0)
	for rows.Next() { var body string; if err := rows.Scan(&body); err != nil { return nil, err }; var value codebase.Document; if err := json.Unmarshal([]byte(body), &value); err != nil { return nil, err }; values = append(values, value) }
	return values, rows.Err()
}
