package cassandra_test

import (
	"strings"
	"testing"

	"github.com/gocql/gocql"

	"github.com/Tangerg/lynx/chathistory/cassandra"
	"github.com/Tangerg/lynx/core/model/chat/history"
)

func stubSession() *gocql.Session { return new(gocql.Session) }

func TestStoreConfig_SessionRequired(t *testing.T) {
	_, err := cassandra.NewStore(cassandra.StoreConfig{})
	if err == nil {
		t.Fatal("expected error when Session is nil")
	}
	if !strings.Contains(err.Error(), "Session") {
		t.Fatalf("err = %v; should mention Session", err)
	}
}

func TestStoreConfig_NilConfig(t *testing.T) {
	if _, err := cassandra.NewStore(cassandra.StoreConfig{}); err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestStoreConfig_RejectsBadIdentifier(t *testing.T) {
	cases := []struct {
		name string
		cfg  cassandra.StoreConfig
	}{
		{"keyspace with hyphen", cassandra.StoreConfig{Session: stubSession(), Keyspace: "my-ks"}},
		{"table with semicolon", cassandra.StoreConfig{Session: stubSession(), TableName: "x;y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cassandra.NewStore(tc.cfg); err == nil {
				t.Fatal("expected identifier-validation error")
			}
		})
	}
}

func TestStoreConfig_AcceptsValidIdentifiers(t *testing.T) {
	_, err := cassandra.NewStore(cassandra.StoreConfig{
		Session:   stubSession(),
		Keyspace:  "lynx",
		TableName: "chat_history",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_ImplementsHistoryStore(t *testing.T) {
	var _ history.Store = (*cassandra.Store)(nil)
}
