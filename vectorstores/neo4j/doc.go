// Package neo4j wraps the official neo4j-go-driver v5 as a
// the vectorstore capability interfaces. Documents become nodes labeled `:Document`
// (or whatever [StoreConfig.Label] picks) — metadata keys are
// stored as flat properties named `metadata.<key>`, the embedding
// rides on the configured property, and the id has a uniqueness
// constraint.
//
// Requirements: Neo4j 5.13+ for `CREATE VECTOR INDEX` and the
// `db.index.vector.queryNodes` procedure. Earlier 5.x releases ship
// the procedure under a different signature; the store hard-codes
// the 5.13+ shape.
//
// Similarity functions: [SimilarityCosine] / [SimilarityEuclidean].
// Both are mapped to a [0, 1] similarity score by Neo4j itself.
//
// Indexing — the store creates two things under
// [StoreConfig.InitializeSchema] = true:
//
//   - a uniqueness constraint on the id property
//   - a `VECTOR INDEX` carrying dimensions + similarity function
//
// Retrieval calls `CALL db.index.vector.queryNodes($index, $k, $vec)
// YIELD node, score WHERE score >= $threshold AND <filter>`. The
// filter visitor produces a Cypher predicate plus a `$pN`-keyed
// parameter map (Cypher uses named parameters).
//
// LIKE maps onto Cypher's `=~` (regex). Note that NOT in Cypher
// must precede an expression — the visitor emits `NOT (<expr>)`.
//
// See https://neo4j.com/docs/cypher-manual/current/indexes-for-vector-search/
// for index syntax and the vector-search reference.
package neo4j
