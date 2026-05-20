package redis_test

import (
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/chatmemory/redis"
	chatmem "github.com/Tangerg/lynx/core/model/chat/memory"
)

func stubClient() goredis.UniversalClient {
	return goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:0"})
}

func TestStoreConfig_ClientRequired(t *testing.T) {
	_, err := redis.NewStore(&redis.StoreConfig{})
	if err == nil {
		t.Fatal("expected error when Client is nil")
	}
	if !strings.Contains(err.Error(), "Client") {
		t.Fatalf("err = %v; should mention Client", err)
	}
}

func TestStoreConfig_NilConfig(t *testing.T) {
	if _, err := redis.NewStore(nil); err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestStoreConfig_NegativeTTL(t *testing.T) {
	_, err := redis.NewStore(&redis.StoreConfig{
		Client: stubClient(),
		TTL:    -1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error on negative TTL")
	}
}

func TestStoreConfig_KeyPrefixDefaults(t *testing.T) {
	if _, err := redis.NewStore(&redis.StoreConfig{Client: stubClient()}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_ImplementsMemoryStore(t *testing.T) {
	var _ chatmem.Store = (*redis.Store)(nil)
}
