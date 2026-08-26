package postgres_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tangerg/lynx/history/postgres"
)

// stubPool is a sentinel non-nil *pgxpool.Pool used to exercise the
// config-validation paths that don't actually issue SQL. The pool is
// never queried — tests that want real I/O need testcontainers or a
// live postgres and live outside the unit suite.
//
// pgxpool.Pool has unexported fields, so we can't construct one
// directly without a real connection. The cheap fix: tests that only
// inspect validation use a hand-built struct via pointer-to-zero.
func stubPool() *pgxpool.Pool { return new(pgxpool.Pool) }

func TestNewRequiresPool(t *testing.T) {
	cfg := postgres.Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Config.Validate should reject a nil Pool")
	}
	_, err := postgres.New(t.Context(), cfg)
	if err == nil {
		t.Fatal("expected error when Pool is nil")
	}
	if !strings.Contains(err.Error(), "pool") {
		t.Fatalf("err = %v; should mention pool", err)
	}
}

func TestNewRejectsBadIdentifier(t *testing.T) {
	cases := []struct {
		name string
		cfg  postgres.Config
	}{
		{
			name: "schema with semicolon",
			cfg:  postgres.Config{Pool: stubPool(), SchemaName: "public; DROP TABLE x"},
		},
		{
			name: "table with hyphen",
			cfg:  postgres.Config{Pool: stubPool(), TableName: "chat history"},
		},
		{
			name: "index starting with digit",
			cfg:  postgres.Config{Pool: stubPool(), IndexName: "1bad"},
		},
		{
			name: "table with space",
			cfg:  postgres.Config{Pool: stubPool(), TableName: "chat history"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := postgres.New(t.Context(), tc.cfg)
			if err == nil {
				t.Fatal("expected identifier-validation error")
			}
			if !strings.Contains(err.Error(), "valid unquoted identifier") {
				t.Fatalf("err = %v; should explain identifier requirement", err)
			}
		})
	}
}

func TestNewAcceptsValidIdentifiers(t *testing.T) {
	// InitializeSchema=false so we don't issue SQL — only validation
	// runs. The stub pool would crash any real query.
	_, err := postgres.New(t.Context(), postgres.Config{
		Pool:       stubPool(),
		SchemaName: "my_schema",
		TableName:  "chat_history",
		IndexName:  "chat_history_lookup",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}
