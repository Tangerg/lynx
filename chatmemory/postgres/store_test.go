package postgres_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	chatmem "github.com/Tangerg/lynx/core/model/chat/memory"
	"github.com/Tangerg/lynx/chatmemory/postgres"
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

func TestStoreConfig_PoolRequired(t *testing.T) {
	_, err := postgres.NewStore(&postgres.StoreConfig{})
	if err == nil {
		t.Fatal("expected error when Pool is nil")
	}
	if !strings.Contains(err.Error(), "Pool") {
		t.Fatalf("err = %v; should mention Pool", err)
	}
}

func TestStoreConfig_NilConfig(t *testing.T) {
	_, err := postgres.NewStore(nil)
	if err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestStoreConfig_RejectsBadIdentifier(t *testing.T) {
	cases := []struct {
		name string
		cfg  *postgres.StoreConfig
	}{
		{
			name: "schema with semicolon",
			cfg:  &postgres.StoreConfig{Pool: stubPool(), SchemaName: "public; DROP TABLE x"},
		},
		{
			name: "table with hyphen",
			cfg:  &postgres.StoreConfig{Pool: stubPool(), TableName: "chat-memory"},
		},
		{
			name: "index starting with digit",
			cfg:  &postgres.StoreConfig{Pool: stubPool(), IndexName: "1bad"},
		},
		{
			name: "table with space",
			cfg:  &postgres.StoreConfig{Pool: stubPool(), TableName: "chat memory"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := postgres.NewStore(tc.cfg)
			if err == nil {
				t.Fatal("expected identifier-validation error")
			}
			if !strings.Contains(err.Error(), "must match") {
				t.Fatalf("err = %v; should mention identifier pattern", err)
			}
		})
	}
}

func TestStoreConfig_AcceptsValidIdentifiers(t *testing.T) {
	// InitializeSchema=false so we don't issue SQL — only validation
	// runs. The stub pool would crash any real query.
	_, err := postgres.NewStore(&postgres.StoreConfig{
		Pool:       stubPool(),
		SchemaName: "my_schema",
		TableName:  "chat_history",
		IndexName:  "chat_history_lookup",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

// TestStore_ImplementsMemoryStore is a compile-time interface check —
// fails at build time, not runtime, if the contract drifts.
func TestStore_ImplementsMemoryStore(t *testing.T) {
	var _ chatmem.Store = (*postgres.Store)(nil)
}
