package sqlite_test

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestMCPServerStoreRoundTrip(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMCPServerStore(db)
	servers := []mcpserver.Server{
		{
			Name:             "files",
			Transport:        mcpserver.TransportStdio,
			Enabled:          true,
			Description:      "local files",
			Command:          "mcp-files",
			Args:             []string{"--root", "/repo"},
			Env:              map[string]string{"TOKEN": "secret"},
			Dir:              "/repo",
			Timeout:          3 * time.Second,
			DisabledTools:    []string{"remove"},
			AutoApproveTools: []string{"read"},
		},
		{
			Name:          "remote",
			Transport:     mcpserver.TransportStreamableHTTP,
			Enabled:       true,
			URL:           "https://mcp.example.test",
			Authorization: "Bearer secret",
			Headers:       map[string]string{"X-Trace": "enabled"},
		},
	}
	for _, want := range servers {
		if err := want.Validate(); err != nil {
			t.Fatalf("invalid fixture %q: %v", want.Name, err)
		}
		if err := store.Configure(t.Context(), want); err != nil {
			t.Fatalf("Configure %q: %v", want.Name, err)
		}

		got, ok, err := store.Get(t.Context(), want.Name)
		if err != nil || !ok {
			t.Fatalf("Get %q: server=%+v ok=%v err=%v", want.Name, got, ok, err)
		}
		if !equalMCPServer(got, want) {
			t.Fatalf("Get %q round trip = %+v, want %+v", want.Name, got, want)
		}
	}
}

func equalMCPServer(a, b mcpserver.Server) bool {
	return a.Name == b.Name && a.Transport == b.Transport && a.Enabled == b.Enabled &&
		a.Description == b.Description && a.URL == b.URL && a.Authorization == b.Authorization &&
		maps.Equal(a.Headers, b.Headers) && a.Command == b.Command && slices.Equal(a.Args, b.Args) &&
		maps.Equal(a.Env, b.Env) && a.Dir == b.Dir && a.Timeout == b.Timeout &&
		slices.Equal(a.DisabledTools, b.DisabledTools) && slices.Equal(a.AutoApproveTools, b.AutoApproveTools)
}

func TestMCPServerStoreRejectsMalformedJSONFields(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMCPServerStore(db)
	server := mcpserver.Server{
		Name:      "files",
		Transport: mcpserver.TransportStdio,
		Enabled:   true,
		Command:   "mcp-files",
	}

	tests := []struct {
		name   string
		update string
		field  string
	}{
		{name: "headers", update: `UPDATE mcp_servers SET headers = '{' WHERE name = ?`, field: "headers"},
		{name: "args", update: `UPDATE mcp_servers SET args = '{' WHERE name = ?`, field: "args"},
		{name: "env", update: `UPDATE mcp_servers SET env = '{' WHERE name = ?`, field: "env"},
		{name: "disabled tools", update: `UPDATE mcp_servers SET disabled_tools = '{' WHERE name = ?`, field: "disabled_tools"},
		{name: "auto approve tools", update: `UPDATE mcp_servers SET auto_approve_tools = '{' WHERE name = ?`, field: "auto_approve_tools"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.Configure(t.Context(), server); err != nil {
				t.Fatalf("Configure: %v", err)
			}
			if _, err := db.ExecContext(t.Context(), test.update, server.Name); err != nil {
				t.Fatalf("corrupt %s: %v", test.field, err)
			}

			if _, ok, err := store.Get(t.Context(), server.Name); err == nil || ok ||
				!strings.Contains(err.Error(), `mcp server "files"`) || !strings.Contains(err.Error(), "decode "+test.field) {
				t.Fatalf("Get malformed %s: ok=%v err=%v", test.field, ok, err)
			}
			if _, err := store.List(t.Context()); err == nil ||
				!strings.Contains(err.Error(), `mcp server "files"`) || !strings.Contains(err.Error(), "decode "+test.field) {
				t.Fatalf("List malformed %s: err=%v", test.field, err)
			}
		})
	}
}
