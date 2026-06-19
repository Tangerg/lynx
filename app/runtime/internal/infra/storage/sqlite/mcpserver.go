package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// MCPServerService implements mcpserver.Service against a SQLite database.
// One row per server name; Configure is an upsert. The []string columns
// (args / env / disabled_tools / auto_approve_tools) are JSON-encoded. The DB
// must have been opened via [Open] so the mcp_servers table exists.
type MCPServerService struct {
	db *sql.DB
}

var _ mcpserver.Service = (*MCPServerService)(nil)

// NewMCPServerService wires the given *sql.DB to the mcpserver.Service surface.
func NewMCPServerService(db *sql.DB) *MCPServerService {
	return &MCPServerService{db: db}
}

func (s *MCPServerService) List(ctx context.Context) ([]mcpserver.Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, transport, enabled, description, url, authorization,
		        command, args, env, dir, disabled_tools, auto_approve_tools
		 FROM mcp_servers ORDER BY name`)
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

func (s *MCPServerService) Get(ctx context.Context, name string) (mcpserver.Server, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT name, transport, enabled, description, url, authorization,
		        command, args, env, dir, disabled_tools, auto_approve_tools
		 FROM mcp_servers WHERE name = ?`, name)
	srv, err := scanMCPServer(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpserver.Server{}, false, nil
	}
	if err != nil {
		return mcpserver.Server{}, false, err
	}
	return srv, true, nil
}

func (s *MCPServerService) Configure(ctx context.Context, srv mcpserver.Server) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mcp_servers
		   (name, transport, enabled, description, url, authorization,
		    command, args, env, dir, disabled_tools, auto_approve_tools)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		    transport = excluded.transport, enabled = excluded.enabled,
		    description = excluded.description, url = excluded.url,
		    authorization = excluded.authorization, command = excluded.command,
		    args = excluded.args, env = excluded.env, dir = excluded.dir,
		    disabled_tools = excluded.disabled_tools,
		    auto_approve_tools = excluded.auto_approve_tools`,
		srv.Name, srv.Transport, srv.Enabled, srv.Description, srv.URL, srv.Authorization,
		srv.Command, encodeStrings(srv.Args), encodeStrings(srv.Env), srv.Dir,
		encodeStrings(srv.DisabledTools), encodeStrings(srv.AutoApproveTools))
	if err != nil {
		return fmt.Errorf("sqlite: configure mcp server: %w", err)
	}
	return nil
}

func (s *MCPServerService) Remove(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM mcp_servers WHERE name = ?`, name); err != nil {
		return fmt.Errorf("sqlite: remove mcp server: %w", err)
	}
	return nil
}

func (s *MCPServerService) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE mcp_servers SET enabled = ? WHERE name = ?`, enabled, name); err != nil {
		return fmt.Errorf("sqlite: set mcp server enabled: %w", err)
	}
	return nil
}

// scanMCPServer reads one row via the given Scan func (works for both
// *sql.Row and *sql.Rows), decoding the JSON []string columns.
func scanMCPServer(scan func(...any) error) (mcpserver.Server, error) {
	var (
		srv                           mcpserver.Server
		args, env, disabled, autoAppr string
	)
	if err := scan(&srv.Name, &srv.Transport, &srv.Enabled, &srv.Description, &srv.URL,
		&srv.Authorization, &srv.Command, &args, &env, &srv.Dir, &disabled, &autoAppr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mcpserver.Server{}, err
		}
		return mcpserver.Server{}, fmt.Errorf("sqlite: scan mcp server: %w", err)
	}
	srv.Args = decodeStrings(args)
	srv.Env = decodeStrings(env)
	srv.DisabledTools = decodeStrings(disabled)
	srv.AutoApproveTools = decodeStrings(autoAppr)
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
