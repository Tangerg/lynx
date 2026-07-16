// Package redis is a chathistory Store backed by Redis via go-redis.
//
// Each conversation maps to a Redis list keyed by
// `<KeyPrefix><conversationID>` (default prefix `chat:history:`).
// Messages are RPUSH'd as canonical [chat.Message] JSON, so a
// LRANGE 0 -1 read recovers the conversation in chronological order.
//
// Example:
//
//	client := goredis.NewUniversalClient(&goredis.UniversalOptions{...})
//	store, _ := redis.New(redis.Config{Client: client})
package redis
