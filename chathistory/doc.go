// Package chathistory defines provider-neutral conversation history contracts,
// reference stores, and persistent database backends.
//
// Reader, Writer, Clearer, and Store use core/chat protocol values directly.
// InMemoryStore is a zero-value-ready reference implementation. WindowStore is
// an explicit read-side retention decorator; optional cross-conversation and
// replacement capabilities remain separate interfaces.
//
// Conversation IDs are runtime scope carried with [WithConversationID], not
// serialized request metadata. The middleware subpackage binds that scope to
// model calls.
//
// Persistent backends live in child packages so database drivers do not enter
// core/go.mod:
//
//	chathistory/postgres/  — PostgreSQL (pgx + JSONB)
//	chathistory/redis/     — Redis (RPUSH / LRANGE lists)
//	chathistory/mongodb/   — MongoDB (document per message)
//	chathistory/cassandra/ — Cassandra (TIMEUUID clustering key)
//	chathistory/neo4j/     — Neo4j (node per message)
//	chathistory/cosmosdb/  — Azure Cosmos DB (NoSQL API)
//
// Every backend reads and writes only the current core/chat tagged JSON wire.
// Historical wire migration is an explicit application data operation, not a
// permanent compatibility branch in the library.
package chathistory
