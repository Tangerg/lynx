// Package redis is a [memory.Store] backed by Redis via go-redis.
//
// Each conversation maps to a Redis list keyed by
// `<KeyPrefix><conversationID>` (default prefix `chat:memory:`).
// Messages are RPUSH'd as canonical [chat.Message] JSON, so a
// LRANGE 0 -1 read recovers the conversation in chronological order.
//
// Example:
//
//	client := goredis.NewUniversalClient(&goredis.UniversalOptions{...})
//	store, _ := redis.NewStore(&redis.StoreConfig{Client: client})
//
//	chatMW, _, _ := memory.NewMiddleware(store)
//	resp, _ := client.Chat().
//	    WithParams(map[string]any{memory.ConversationIDKey: "u-42"}).
//	    WithMiddlewares(chatMW).
//	    WithUserPrompt("hi").
//	    Call().Response(ctx)
package redis
