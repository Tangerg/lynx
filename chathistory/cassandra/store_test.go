package cassandra_test

import (
	"strings"
	"testing"

	"github.com/gocql/gocql"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/cassandra"
)

func stubSession() *gocql.Session { return new(gocql.Session) }

func TestNewRequiresSession(t *testing.T) {
	_, err := cassandra.New(cassandra.Config{})
	if err == nil {
		t.Fatal("expected error when Session is nil")
	}
	if !strings.Contains(err.Error(), "Session") {
		t.Fatalf("err = %v; should mention Session", err)
	}
}

func TestNewRejectsBadIdentifier(t *testing.T) {
	cases := []struct {
		name string
		cfg  cassandra.Config
	}{
		{"keyspace with hyphen", cassandra.Config{Session: stubSession(), Keyspace: "my-ks"}},
		{"table with semicolon", cassandra.Config{Session: stubSession(), TableName: "x;y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cassandra.New(tc.cfg); err == nil {
				t.Fatal("expected identifier-validation error")
			}
		})
	}
}

func TestNewAcceptsValidIdentifiers(t *testing.T) {
	_, err := cassandra.New(cassandra.Config{
		Session:   stubSession(),
		Keyspace:  "lynx",
		TableName: "chat_history",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*cassandra.Store)(nil)
}
