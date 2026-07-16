package redis_test

import (
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/redis"
)

func stubClient() goredis.UniversalClient {
	return goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:0"})
}

func TestNewRequiresClient(t *testing.T) {
	_, err := redis.New(redis.Config{})
	if err == nil {
		t.Fatal("expected error when Client is nil")
	}
	if !strings.Contains(err.Error(), "Client") {
		t.Fatalf("err = %v; should mention Client", err)
	}
}

func TestNewRejectsNegativeTTL(t *testing.T) {
	_, err := redis.New(redis.Config{
		Client: stubClient(),
		TTL:    -1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error on negative TTL")
	}
}

func TestNewDefaultsKeyPrefix(t *testing.T) {
	if _, err := redis.New(redis.Config{Client: stubClient()}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*redis.Store)(nil)
}
