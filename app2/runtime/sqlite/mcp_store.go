package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/mcpserver"
)

type storedMCPServer struct {
	Enabled          bool                `json:"enabled"`
	Description      string              `json:"description,omitempty"`
	Transport        mcpserver.Transport `json:"transport"`
	URL              string              `json:"url,omitempty"`
	Command          string              `json:"command,omitempty"`
	Args             []string            `json:"args,omitempty"`
	Dir              string              `json:"dir,omitempty"`
	TimeoutSeconds   int                 `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string            `json:"disabledTools,omitempty"`
	AutoApproveTools []string            `json:"autoApproveTools,omitempty"`
}

type storedMCPAuthorizationAttempt struct {
	Status     mcpserver.AuthorizationStatus `json:"status"`
	FinishedAt *time.Time                    `json:"finishedAt,omitempty"`
}

func (database *Database) ListMCPServers(ctx context.Context) ([]mcpserver.Configuration, error) {
	rows, err := database.database.QueryContext(ctx, `
		SELECT id, body, COALESCE(secret, x''), revision, updated_at
		FROM mcp_servers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list MCP servers: %w", err)
	}
	defer rows.Close()
	values := make([]mcpserver.Configuration, 0)
	for rows.Next() {
		value, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) GetMCPServer(ctx context.Context, name string) (mcpserver.Configuration, error) {
	value, err := scanMCPServer(database.database.QueryRowContext(ctx, `
		SELECT id, body, COALESCE(secret, x''), revision, updated_at
		FROM mcp_servers WHERE id = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.Configuration{}, mcpserver.ErrNotFound
	}
	if err != nil {
		return mcpserver.Configuration{}, fmt.Errorf("sqlite: get MCP server %q: %w", name, err)
	}
	return value, nil
}

func (database *Database) SaveMCPServer(
	ctx context.Context,
	value mcpserver.Configuration,
	previousRevision uint64,
) error {
	body, secret, err := encodeMCPServer(value)
	if err != nil {
		return err
	}
	var result sql.Result
	if previousRevision == 0 {
		result, err = database.database.ExecContext(ctx, `
			INSERT INTO mcp_servers (id, body, secret, revision, updated_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`,
			value.Name(), body, secret, value.Revision(), encodeTime(value.UpdatedAt()))
	} else {
		result, err = database.database.ExecContext(ctx, `
			UPDATE mcp_servers
			SET body = ?, secret = ?, revision = ?, updated_at = ?
			WHERE id = ? AND revision = ?`,
			body, secret, value.Revision(), encodeTime(value.UpdatedAt()),
			value.Name(), previousRevision)
	}
	if err != nil {
		return fmt.Errorf("sqlite: save MCP server %q: %w", value.Name(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect MCP server save %q: %w", value.Name(), err)
	}
	if changed == 0 {
		if previousRevision == 0 {
			return mcpserver.ErrExists
		}
		return mcpserver.ErrRevisionConflict
	}
	return nil
}

func (database *Database) DeleteMCPServer(ctx context.Context, name string, expectedRevision uint64) error {
	result, err := database.database.ExecContext(ctx, `
		DELETE FROM mcp_servers WHERE id = ? AND revision = ?`, name, expectedRevision)
	if err != nil {
		return fmt.Errorf("sqlite: delete MCP server %q: %w", name, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect MCP server delete %q: %w", name, err)
	}
	if changed != 0 {
		return nil
	}
	var exists int
	err = database.database.QueryRowContext(ctx, `SELECT 1 FROM mcp_servers WHERE id = ?`, name).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.ErrNotFound
	}
	if err != nil {
		return err
	}
	return mcpserver.ErrRevisionConflict
}

func (database *Database) PutMCPAuthorizationAttempt(
	ctx context.Context,
	value mcpserver.AuthorizationAttempt,
) error {
	return putMCPAuthorizationAttempt(ctx, database.database, value)
}

func (database *Database) GetMCPAuthorizationAttempt(
	ctx context.Context,
	id string,
) (mcpserver.AuthorizationAttempt, error) {
	var server, body, createdAt string
	err := database.database.QueryRowContext(ctx, `
		SELECT server_id, body, created_at
		FROM mcp_authorization_attempts WHERE id = ?`, id,
	).Scan(&server, &body, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.AuthorizationAttempt{}, mcpserver.ErrNotFound
	}
	if err != nil {
		return mcpserver.AuthorizationAttempt{}, err
	}
	return decodeMCPAuthorizationAttempt(id, server, body, createdAt)
}

// RecoverMCPAuthorizationAttempts closes attempts owned by the predecessor
// process and prunes only terminal outcomes older than the published retention.
func (database *Database) RecoverMCPAuthorizationAttempts(
	ctx context.Context,
	now time.Time,
	cutoff time.Time,
) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	rows, err := transaction.QueryContext(ctx, `
		SELECT id, server_id, body, created_at
		FROM mcp_authorization_attempts
		WHERE json_extract(body, '$.status') = 'pending'`)
	if err != nil {
		return err
	}
	var pending []mcpserver.AuthorizationAttempt
	for rows.Next() {
		var id, server, body, createdAt string
		if err := rows.Scan(&id, &server, &body, &createdAt); err != nil {
			rows.Close()
			return err
		}
		value, err := decodeMCPAuthorizationAttempt(id, server, body, createdAt)
		if err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for index := range pending {
		if err := pending[index].Finish(mcpserver.AuthorizationCanceled, now); err != nil {
			return err
		}
		if err := putMCPAuthorizationAttempt(ctx, transaction, pending[index]); err != nil {
			return err
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM mcp_authorization_attempts
		WHERE json_extract(body, '$.status') != 'pending'
			AND julianday(updated_at) <= julianday(?)`,
		encodeTime(cutoff)); err != nil {
		return err
	}
	return transaction.Commit()
}

func (database *Database) PruneMCPAuthorizationAttempts(ctx context.Context, cutoff time.Time) error {
	_, err := database.database.ExecContext(ctx, `
		DELETE FROM mcp_authorization_attempts
		WHERE json_extract(body, '$.status') != 'pending'
			AND julianday(updated_at) <= julianday(?)`,
		encodeTime(cutoff))
	return err
}

type mcpServerScanner interface {
	Scan(...any) error
}

func scanMCPServer(scanner mcpServerScanner) (mcpserver.Configuration, error) {
	var name, body, updatedAt string
	var secret []byte
	var revision uint64
	if err := scanner.Scan(&name, &body, &secret, &revision, &updatedAt); err != nil {
		return mcpserver.Configuration{}, err
	}
	var stored storedMCPServer
	if err := decodeMCPJSON([]byte(body), &stored); err != nil {
		return mcpserver.Configuration{}, fmt.Errorf("sqlite: decode MCP server %q: %w", name, err)
	}
	var secrets mcpserver.SecretState
	if len(secret) > 0 {
		if err := decodeMCPJSON(secret, &secrets); err != nil {
			return mcpserver.Configuration{}, fmt.Errorf("sqlite: decode MCP secrets %q: %w", name, err)
		}
	}
	updated, err := decodeTime(updatedAt)
	if err != nil {
		return mcpserver.Configuration{}, err
	}
	value, err := mcpserver.Rehydrate(mcpserver.State{
		Name: name, Enabled: stored.Enabled, Description: stored.Description,
		Transport: stored.Transport, URL: stored.URL, Command: stored.Command,
		Args: slices.Clone(stored.Args), Dir: stored.Dir,
		TimeoutSeconds: stored.TimeoutSeconds,
		DisabledTools: slices.Clone(stored.DisabledTools),
		AutoApproveTools: slices.Clone(stored.AutoApproveTools),
		Secrets: secrets, Revision: revision, UpdatedAt: updated,
	})
	if err != nil {
		return mcpserver.Configuration{}, fmt.Errorf("sqlite: restore MCP server %q: %w", name, err)
	}
	return value, nil
}

func encodeMCPServer(value mcpserver.Configuration) (string, []byte, error) {
	state := value.State()
	body, err := json.Marshal(storedMCPServer{
		Enabled: state.Enabled, Description: state.Description, Transport: state.Transport,
		URL: state.URL, Command: state.Command, Args: state.Args, Dir: state.Dir,
		TimeoutSeconds: state.TimeoutSeconds, DisabledTools: state.DisabledTools,
		AutoApproveTools: state.AutoApproveTools,
	})
	if err != nil {
		return "", nil, err
	}
	secret, err := json.Marshal(state.Secrets)
	if err != nil {
		return "", nil, err
	}
	return string(body), secret, nil
}

type mcpAttemptExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func putMCPAuthorizationAttempt(
	ctx context.Context,
	execer mcpAttemptExecer,
	value mcpserver.AuthorizationAttempt,
) error {
	state := value.State()
	body, err := json.Marshal(storedMCPAuthorizationAttempt{
		Status: state.Status, FinishedAt: state.FinishedAt,
	})
	if err != nil {
		return err
	}
	updatedAt := state.CreatedAt
	if state.FinishedAt != nil {
		updatedAt = *state.FinishedAt
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO mcp_authorization_attempts
			(id, server_id, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
		state.ID, state.Server, string(body), encodeTime(state.CreatedAt), encodeTime(updatedAt))
	return err
}

func decodeMCPAuthorizationAttempt(
	id string,
	server string,
	body string,
	createdAt string,
) (mcpserver.AuthorizationAttempt, error) {
	var stored storedMCPAuthorizationAttempt
	if err := decodeMCPJSON([]byte(body), &stored); err != nil {
		return mcpserver.AuthorizationAttempt{}, err
	}
	created, err := decodeTime(createdAt)
	if err != nil {
		return mcpserver.AuthorizationAttempt{}, err
	}
	return mcpserver.RehydrateAuthorizationAttempt(mcpserver.AuthorizationAttemptState{
		ID: id, Server: server, Status: stored.Status,
		CreatedAt: created, FinishedAt: stored.FinishedAt,
	})
}

func decodeMCPJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("sqlite: MCP record contains trailing JSON")
		}
		return err
	}
	return nil
}
