// Package redis is a history Store backed by Redis via go-redis.
//
// Each conversation maps to a Redis list keyed by
// `<KeyPrefix><conversationID>` (default prefix `chat:history:`).
// Messages are RPUSH'd as canonical [chat.Message] JSON, so a
// LRANGE 0 -1 preserves list order. When TTL is configured, append and expiry
// refresh execute in one Redis transaction.
//
// Example:
//
//	client := goredis.NewUniversalClient(&goredis.UniversalOptions{...})
//	store, _ := redis.NewStore(redis.StoreConfig{Client: client})
package redis
