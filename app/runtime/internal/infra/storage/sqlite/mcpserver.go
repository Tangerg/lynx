package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// MCPServerStore implements mcpserver.Registry against a SQLite database.
// One row per server name; Configure is an upsert. The list columns (args /
// disabled_tools / auto_approve_tools) and the map columns (env / headers) are
// JSON-encoded; timeout is stored as nanoseconds. The DB must have been opened
// via [Open] so the mcp_servers table exists.
type MCPServerStore struct {
	db *sql.DB
}

var _ mcpserver.Registry = (*MCPServerStore)(nil)

// NewMCPServerStore wires the given *sql.DB to the mcpserver.Registry surface.
func NewMCPServerStore(db *sql.DB) *MCPServerStore {
	return &MCPServerStore{db: db}
}

// mcpColumns is the column list shared by List and Get so the two reads and
// scanMCPServer stay in lockstep.
const mcpColumns = `name, transport, enabled, description, url, authorization, headers,
	        command, args, env, dir, timeout, disabled_tools, auto_approve_tools`

func (s *MCPServerStore) List(ctx context.Context) ([]mcpserver.Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+mcpColumns+` FROM mcp_servers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list mcp servers: %w", err)
	}
	defer rows.Close()

	var out []mcpserver.Server
	for rows.Next() {
		srv, scanErr := scanMCPServer(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *MCPServerStore) Get(ctx context.Context, name string) (mcpserver.Server, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+mcpColumns+` FROM mcp_servers WHERE name = ?`, name)
	srv, err := scanMCPServer(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.Server{}, false, nil
	}
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	return srv, true, nil
}

func (s *MCPServerStore) Configure(ctx context.Context, srv mcpserver.Server) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mcp_servers
		   (name, transport, enabled, description, url, authorization, headers,
		    command, args, env, dir, timeout, disabled_tools, auto_approve_tools)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		    transport = excluded.transport, enabled = excluded.enabled,
		    description = excluded.description, url = excluded.url,
		    authorization = excluded.authorization, headers = excluded.headers,
		    command = excluded.command, args = excluded.args, env = excluded.env,
		    dir = excluded.dir, timeout = excluded.timeout,
		    disabled_tools = excluded.disabled_tools,
		    auto_approve_tools = excluded.auto_approve_tools`,
		srv.Name, srv.Transport, srv.Enabled, srv.Description, srv.URL, srv.Authorization,
		encodeStringMap(srv.Headers), srv.Command, encodeStrings(srv.Args),
		encodeStringMap(srv.Env), srv.Dir, int64(srv.Timeout),
		encodeStrings(srv.DisabledTools), encodeStrings(srv.AutoApproveTools))
	if err != nil {
		return fmt.Errorf("sqlite: configure mcp server: %w", err)
	}
	return nil
}

func (s *MCPServerStore) Remove(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE name = ?`, name); err != nil {
		return fmt.Errorf("sqlite: remove mcp server: %w", err)
	}
	return nil
}

func (s *MCPServerStore) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE mcp_servers SET enabled = ? WHERE name = ?`, enabled, name); err != nil {
		return fmt.Errorf("sqlite: set mcp server enabled: %w", err)
	}
	return nil
}

// scanMCPServer reads one row via the given Scan func (works for both
// *sql.Row and *sql.Rows), decoding the JSON list/map columns and the
// nanosecond timeout. Column order must match [mcpColumns].
func scanMCPServer(scan func(...any) error) (mcpserver.Server, error) {
	var (
		srv                                 mcpserver.Server
		headers, args, env, disabled, autoA string
		timeoutNS                           int64
	)
	if err := scan(&srv.Name, &srv.Transport, &srv.Enabled, &srv.Description, &srv.URL,
		&srv.Authorization, &headers, &srv.Command, &args, &env, &srv.Dir, &timeoutNS,
		&disabled, &autoA); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcpserver.Server{}, err
		}
		return mcpserver.Server{}, fmt.Errorf("sqlite: scan mcp server: %w", err)
	}
	srv.Headers = decodeStringMap(headers)
	srv.Args = decodeStrings(args)
	srv.Env = decodeStringMap(env)
	srv.Timeout = time.Duration(timeoutNS)
	srv.DisabledTools = decodeStrings(disabled)
	srv.AutoApproveTools = decodeStrings(autoA)
	return srv, nil
}

// encodeStrings JSON-encodes a string slice for a TEXT column; a nil/empty
// slice stores "" (decoded back to nil) so empty and absent read identically.
func encodeStrings(v []string) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeStrings reverses encodeStrings; a blank or malformed column yields nil.
func decodeStrings(s string) []string {
	if s == "" {
		return nil
	}
	var v []string
	if json.Unmarshal([]byte(s), &v) != nil {
		return nil
	}
	return v
}

// encodeStringMap JSON-encodes a string map for a TEXT column; a nil/empty map
// stores "" (decoded back to nil) so empty and absent read identically.
func encodeStringMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeStringMap reverses encodeStringMap; a blank or malformed column yields nil.
func decodeStringMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	var m map[string]string
	if json.Unmarshal([]byte(s), &m) != nil {
		return nil
	}
	return m
}
