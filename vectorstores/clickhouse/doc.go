// Package clickhouse wraps ClickHouse vector similarity search as a
// the vectorstore capability interfaces. Documents live in a MergeTree table
// (id / content / metadata Map(String,String) / embedding
// Array(Float32)) reached through the official clickhouse-go v2
// driver.
//
// Requirements: ClickHouse 24.x+ for the vector_similarity index
// type (HNSW-backed). Earlier ClickHouse versions can still run the
// store — they fall back to exhaustive `cosineDistance` /
// `L2Distance` scans without an ANN index, which is still useful
// for analytics-scale rather than real-time RAG.
//
// Distance metrics: [DistanceCosine] (uses `cosineDistance`) /
// [DistanceL2] (uses `L2Distance`). The store also wires the
// matching index distance parameter into the `vector_similarity`
// index definition.
//
// Metadata model. ClickHouse has no native JSON-path operator;
// metadata is a `Map(String, String)` and accessed via subscript
// (`metadata['key']`). The filter visitor reaches into it directly
// — numeric / ordering comparisons wrap the subscript in
// `toFloat64OrZero(...)` so range queries work.
//
// Insert path. Uses the typed batch API (`Conn.PrepareBatch` +
// `Batch.Append` + `Batch.Send`) — efficient for the bulk-insert
// shape ClickHouse expects.
//
// Delete uses `ALTER TABLE ... DELETE WHERE`, which is an
// asynchronous mutation; callers needing sync semantics should set
// the appropriate connection setting (`mutations_sync = 1` or 2).
//
// See https://clickhouse.com/docs/en/engines/table-engines/
// mergetree-family/annindexes for the official reference.
package clickhouse
