// Package chatmemory hosts persistent backends for the
// [github.com/Tangerg/lynx/core/model/chat/memory.Store] interface.
//
// The abstraction itself, plus the zero-dependency [InMemoryStore] and
// [MessageWindowStore] defaults, lives in core/model/chat/memory and
// does not move. This module exists so each persistent backend can
// pull in its own database driver (pgx, go-redis, mongo-driver, …)
// without polluting core/go.mod.
//
// The topology mirrors vectorstores/: core/ has the interface,
// chatmemory/ siblings have the implementations.
//
//	core/model/chat/memory/  — Store / Reader / Writer / Clearer
//	                           + InMemoryStore + MessageWindowStore (default)
//	chatmemory/postgres/     — PostgreSQL (pgx + JSONB)
//	chatmemory/redis/        — Redis (RPUSH / LRANGE lists)
//	chatmemory/mongodb/      — MongoDB (document per message)
//	chatmemory/cassandra/    — Cassandra (TIMEUUID clustering key)
//	chatmemory/neo4j/        — Neo4j (node per message)
//	chatmemory/cosmosdb/     — Azure Cosmos DB (NoSQL API)
//
// Every backend round-trips messages through the canonical
// [chat.Message] JSON shape ([chat.UnmarshalMessage] / each message
// type's MarshalJSON), so the same conversation can be read back
// after a restart with full fidelity — including AssistantMessage
// Parts ordering, ToolMessage ToolReturns, and metadata maps.
package chatmemory
