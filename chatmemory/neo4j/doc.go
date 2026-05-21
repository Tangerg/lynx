// Package neo4j is a [memory.Store] backed by Neo4j via the official
// Go driver (v5).
//
// Storage model:
//
//	(:ChatMessage {
//	    conversation_id: "u-42",
//	    seq:             <int64 nanos>,
//	    message:         "<json>",
//	    created_at:      <datetime>
//	})
//
// A composite index on (`conversation_id`, `seq`) is created by
// InitializeSchema=true so reads stream in insertion order without a
// full collection scan. `seq` is a Go-side nanosecond timestamp; the
// batch-offset is added to ensure messages from one Write call are
// strictly ordered even when nanoseconds happen to collide.
//
// Example:
//
//	drv, _ := neo4j.NewDriverWithContext("neo4j://...", auth)
//	defer drv.Close(ctx)
//	store, _ := neo4jmem.NewStore(&neo4jmem.StoreConfig{
//	    Driver:           drv,
//	    Database:         "neo4j",
//	    InitializeSchema: true,
//	})
package neo4j
