// Package postgres is a [memory.Store] backed by PostgreSQL via pgx.
//
// Each conversation's messages live in a single table; messages are
// serialized to JSONB via the canonical [chat.Message] JSON shape and
// reassembled on read with [chat.UnmarshalMessage], so AssistantMessage
// Parts ordering, ToolMessage ToolReturns, and metadata maps all
// round-trip with full fidelity.
//
// Example:
//
//	pool, _ := pgxpool.New(ctx, "postgres://...")
//	store, _ := postgres.NewStore(postgres.StoreConfig{
//	    Pool:             pool,
//	    InitializeSchema: true, // create the table+index on first use
//	})
//	defer pool.Close()
//
//	chatMW, _, _ := memory.NewMiddleware(store)
//	resp, _ := client.Chat().
//	    WithParams(map[string]any{chat.ConversationIDKey: "u-42"}).
//	    WithMiddlewares(chatMW).
//	    WithUserPrompt("hi").
//	    Call().Response(ctx)
package postgres
