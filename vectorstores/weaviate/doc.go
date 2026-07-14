// Package weaviate wraps Weaviate as a the vectorstore capability interfaces.
// Documents are stored as objects in a Weaviate class (`{id,
// vector, properties}`); retrieval runs Weaviate's `nearVector`
// (or `nearText` when the class is configured with a vectorizer)
// query.
//
// Requirements: a reachable Weaviate v5 server (self-hosted or
// Weaviate Cloud Services). The store uses the official
// weaviate-go-client/v5.
//
// Vector similarity functions: cosine / dot / l2-squared / hamming
// / manhattan. The chosen value is bound to the class's vector
// index config at creation time.
//
// Schema. Weaviate is strongly typed — properties participating in
// filters must be declared at class-creation time. [StoreConfig]
// enumerates these properties so the store can issue a CREATE
// CLASS when needed.
//
// Filter visitor produces Weaviate's `where` filter operator tree
// — `{"operator": "Equal", "path": ["author"], "valueText": "..."}`,
// `{"operator": "And", "operands": [...]}`, `{"operator":
// "GreaterThan", "valueNumber": 100}`. The result feeds the
// `WithWhere` builder on the GraphQL Get call.
//
// See https://weaviate.io/developers/weaviate for the full API
// surface.
package weaviate
