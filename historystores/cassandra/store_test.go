package cassandra_test

import (
	"strings"
	"testing"

	"github.com/gocql/gocql"

	"github.com/Tangerg/scope/historystores/cassandra"
)

func stubSession() *gocql.Session { return new(gocql.Session) }

func TestNewStoreRequiresSession(t *testing.T) {
	config := cassandra.StoreConfig{}
	if err := config.Validate(); err == nil {
		t.Fatal("StoreConfig.Validate should reject a nil Session")
	}
	_, err := cassandra.NewStore(t.Context(), config)
	if err == nil {
		t.Fatal("expected error when Session is nil")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Fatalf("err = %v; should mention session", err)
	}
}

func TestNewStoreRejectsBadIdentifier(t *testing.T) {
	cases := []struct {
		name   string
		config cassandra.StoreConfig
	}{
		{"keyspace with hyphen", cassandra.StoreConfig{Session: stubSession(), Keyspace: "my-ks"}},
		{"table with semicolon", cassandra.StoreConfig{Session: stubSession(), TableName: "x;y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cassandra.NewStore(t.Context(), tc.config); err == nil {
				t.Fatal("expected identifier-validation error")
			}
		})
	}
}

func TestNewStoreAcceptsValidIdentifiers(t *testing.T) {
	_, err := cassandra.NewStore(t.Context(), cassandra.StoreConfig{
		Session:   stubSession(),
		Keyspace:  "scope",
		TableName: "chat_history",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
