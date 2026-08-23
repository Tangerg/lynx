package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/mcpserver"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (database *Database) ListMCPServerRecords(ctx context.Context) ([]mcpserver.Record, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body FROM mcp_servers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list mcp servers: %w", err)
	}
	defer rows.Close()
	values := make([]mcpserver.Record, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var value mcpserver.Record
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, fmt.Errorf("sqlite: decode mcp server: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) GetMCPServerRecord(ctx context.Context, name string) (mcpserver.Record, mcpserver.Secrets, error) {
	var body string
	var secret []byte
	err := database.database.QueryRowContext(ctx, `SELECT body, COALESCE(secret, x'') FROM mcp_servers WHERE id = ?`, name).Scan(&body, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.Record{}, mcpserver.Secrets{}, mcpserver.ErrNotFound
	}
	if err != nil {
		return mcpserver.Record{}, mcpserver.Secrets{}, fmt.Errorf("sqlite: get mcp server: %w", err)
	}
	var record mcpserver.Record
	if err := json.Unmarshal([]byte(body), &record); err != nil {
		return mcpserver.Record{}, mcpserver.Secrets{}, err
	}
	var secrets mcpserver.Secrets
	if len(secret) > 0 {
		if err := json.Unmarshal(secret, &secrets); err != nil {
			return mcpserver.Record{}, mcpserver.Secrets{}, fmt.Errorf("sqlite: decode mcp secrets: %w", err)
		}
	}
	return record, secrets, nil
}

func (database *Database) CreateMCPServerRecord(ctx context.Context, record mcpserver.Record, secrets mcpserver.Secrets) error {
	body, secret, err := encodeMCPRecord(record, secrets)
	if err != nil {
		return err
	}
	_, err = database.database.ExecContext(ctx, `INSERT INTO mcp_servers (id, body, secret, updated_at) VALUES (?, ?, ?, ?)`,
		record.Name, body, secret, encodeTime(record.UpdatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: create mcp server: %w", err)
	}
	return nil
}

func (database *Database) PutMCPServerRecord(ctx context.Context, record mcpserver.Record, secrets mcpserver.Secrets) error {
	body, secret, err := encodeMCPRecord(record, secrets)
	if err != nil {
		return err
	}
	result, err := database.database.ExecContext(ctx, `UPDATE mcp_servers SET body=?, secret=?, updated_at=? WHERE id=?`,
		body, secret, encodeTime(record.UpdatedAt), record.Name)
	if err != nil {
		return fmt.Errorf("sqlite: update mcp server: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return mcpserver.ErrNotFound
	}
	return nil
}

func (database *Database) DeleteMCPServerRecord(ctx context.Context, name string) error {
	result, err := database.database.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id = ?`, name)
	if err != nil {
		return fmt.Errorf("sqlite: delete mcp server: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return mcpserver.ErrNotFound
	}
	return nil
}

// PutMCPOAuthSession changes only the secret-bearing OAuth projection of the
// latest server record. This avoids overwriting concurrent non-secret server
// edits made while an interactive authorization flow was open in the browser.
func (database *Database) PutMCPOAuthSession(ctx context.Context, name, origin string, payload []byte) error {
	if origin == "" || len(payload) == 0 {
		return errors.New("sqlite: OAuth origin and session are required")
	}
	return database.changeMCPOAuthSession(ctx, name, func(record *mcpserver.Record, secrets *mcpserver.Secrets) error {
		currentOrigin, err := mcpHTTPOrigin(record.URL)
		if err != nil || currentOrigin != origin {
			return errors.New("sqlite: OAuth session origin does not match MCP server")
		}
		secrets.Authorization = ""
		for key := range secrets.Headers {
			if strings.EqualFold(key, "Authorization") {
				delete(secrets.Headers, key)
			}
		}
		secrets.OAuthOrigin = origin
		secrets.OAuthSession = bytes.Clone(payload)
		record.AuthorizationSet = false
		record.HeaderNames = sortedStringKeys(secrets.Headers)
		record.OAuthSet = true
		return nil
	})
}

func (database *Database) ClearMCPOAuthSession(ctx context.Context, name string) error {
	return database.changeMCPOAuthSession(ctx, name, func(record *mcpserver.Record, secrets *mcpserver.Secrets) error {
		secrets.OAuthOrigin = ""
		secrets.OAuthSession = nil
		record.OAuthSet = false
		return nil
	})
}

func (database *Database) changeMCPOAuthSession(ctx context.Context, name string, change func(*mcpserver.Record, *mcpserver.Secrets) error) error {
	tx, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var body string
	var secret []byte
	if err := tx.QueryRowContext(ctx, `SELECT body, COALESCE(secret, x'') FROM mcp_servers WHERE id = ?`, name).Scan(&body, &secret); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcpserver.ErrNotFound
		}
		return err
	}
	var record mcpserver.Record
	var secrets mcpserver.Secrets
	if err := json.Unmarshal([]byte(body), &record); err != nil {
		return err
	}
	if len(secret) > 0 {
		if err := json.Unmarshal(secret, &secrets); err != nil {
			return err
		}
	}
	if err := change(&record, &secrets); err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	encodedBody, encodedSecret, err := encodeMCPRecord(record, secrets)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE mcp_servers SET body=?, secret=?, updated_at=? WHERE id=?`, encodedBody, encodedSecret, encodeTime(record.UpdatedAt), name)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return mcpserver.ErrNotFound
	}
	return tx.Commit()
}

func (database *Database) PutMCPAuthorizationAttempt(ctx context.Context, value protocol.MCPAuthorizationAttempt) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = database.database.ExecContext(ctx, `
		INSERT INTO mcp_authorization_attempts (id, server_id, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at`,
		value.ID, value.Server, string(body), encodeTime(value.CreatedAt), encodeTime(timeOr(value.FinishedAt, value.CreatedAt)))
	return err
}

func (database *Database) GetMCPAuthorizationAttempt(ctx context.Context, id string) (protocol.MCPAuthorizationAttempt, error) {
	var body string
	err := database.database.QueryRowContext(ctx, `SELECT body FROM mcp_authorization_attempts WHERE id = ?`, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.MCPAuthorizationAttempt{}, mcpserver.ErrNotFound
	}
	if err != nil {
		return protocol.MCPAuthorizationAttempt{}, err
	}
	var value protocol.MCPAuthorizationAttempt
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return protocol.MCPAuthorizationAttempt{}, err
	}
	return value, nil
}

func encodeMCPRecord(record mcpserver.Record, secrets mcpserver.Secrets) (string, []byte, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return "", nil, err
	}
	secret, err := json.Marshal(secrets)
	if err != nil {
		return "", nil, err
	}
	return string(body), secret, nil
}

func timeOr(value *time.Time, fallback time.Time) time.Time {
	if value != nil {
		return *value
	}
	return fallback
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func mcpHTTPOrigin(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid MCP HTTP endpoint")
	}
	return (&url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: strings.ToLower(parsed.Host)}).String(), nil
}
